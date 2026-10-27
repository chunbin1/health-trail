package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/lee/health-trail/internal/mcpserver"
	"github.com/lee/health-trail/internal/store"
)

func TestNormalizeBaseURL(t *testing.T) {
	ok, err := NormalizeBaseURL("https://api.example.com/v1/chat/completions")
	if err != nil || ok != "https://api.example.com/v1" {
		t.Fatalf("%q %v", ok, err)
	}
	if _, err := NormalizeBaseURL("http://api.example.com/v1"); err == nil {
		t.Fatal("http to a remote host was accepted")
	}
	if _, err := NormalizeBaseURL("http://127.0.0.1:8080/v1/"); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeBaseURL("https://user:pw@api.example.com/v1"); err == nil {
		t.Fatal("credentials were accepted")
	}
	if _, err := NormalizeBaseURL("file:///tmp/x"); err == nil {
		t.Fatal("file URL was accepted")
	}
}

func TestConfigKeepsKey(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	url := "https://api.example.com/v1"
	key := "sk-test-secret"
	model := "gpt-test"
	c, err = c.Update(&url, &key, &model, false)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Configured() {
		t.Fatal("not configured")
	}
	fi, err := os.Stat(c.path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
	raw, _ := os.ReadFile(c.path)
	if !strings.Contains(string(raw), key) {
		t.Fatal("key not stored")
	}
	empty := ""
	c, err = c.Update(nil, &empty, nil, false)
	if err != nil || c.APIKey != key {
		t.Fatalf("empty key should keep the old one: %q %v", c.APIKey, err)
	}
	c, err = c.Update(nil, nil, nil, true)
	if err != nil || c.APIKey != "" || c.Configured() {
		t.Fatalf("clear: %+v %v", c, err)
	}
}

// fakeModel is an OpenAI-compatible service that answers each request with
// the next reply in order and keeps what it was sent.
type fakeModel struct {
	t       *testing.T
	replies []string
	bodies  []string
	auth    []string
}

func (f *fakeModel) start() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			f.t.Errorf("path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		f.bodies = append(f.bodies, string(b))
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		n := len(f.bodies) - 1
		if n >= len(f.replies) {
			n = len(f.replies) - 1
		}
		io.WriteString(w, f.replies[n])
	}))
	f.t.Cleanup(srv.Close)
	return srv
}

func (f *fakeModel) cfg(srv *httptest.Server) Config {
	return Config{BaseURL: srv.URL + "/v1", APIKey: "sk-secret", Model: "m"}
}

// calls is a reply asking for tools; each pair is a name and its arguments.
func calls(pairs ...string) string { return callsSaying("", pairs...) }

// callsSaying is calls with text next to the tool calls.
func callsSaying(text string, pairs ...string) string {
	var tc []map[string]any
	for i := 0; i+1 < len(pairs); i += 2 {
		tc = append(tc, map[string]any{
			"id": "call_" + string(rune('a'+i/2)), "type": "function",
			"function": map[string]any{"name": pairs[i], "arguments": pairs[i+1]},
		})
	}
	b, _ := json.Marshal(map[string]any{
		"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110, "prompt_tokens_details": map[string]any{"cached_tokens": 80}},
		"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": nullable(text), "tool_calls": tc, "reasoning_content": "想一想"}}},
	})
	return string(b)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func answer(text string) string {
	b, _ := json.Marshal(map[string]any{
		"usage":   map[string]any{"prompt_tokens": 120, "completion_tokens": 20, "total_tokens": 140},
		"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": text}}},
	})
	return string(b)
}

var day = time.Date(2026, 10, 27, 9, 30, 0, 0, time.Local)

func TestReadsWithMCPToolsAndDraftsWrites(t *testing.T) {
	st := openStore(t)
	if _, err := st.Create("conditions", store.Record{"name": "胃炎", "status": "进行中"}); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ date, v string }{{"2019-03-02", "520↑"}, {"2026-10-20", "430"}} {
		if _, err := mcpserver.ImportLabs(st, "", store.Record{"report_date": r.date, "title": "生化", "report_type": "化验"}, "",
			[]store.Record{{"item_name": "尿酸", "value_text": r.v, "unit": "μmol/L", "ref_range": "208-428", "test_date": r.date}}); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeModel{t: t, replies: []string{
		calls("get_lab_trend", `{"item_name":"尿酸"}`,
			"add_record", `{"type":"symptoms","fields":{"occurred_at":"2026-10-27 09:00","description":"头痛","severity":4}}`),
		answer("尿酸 2019 年偏高，今年回到 430。头痛整理好了，请核对。"),
	}}
	var heard []Step
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "今天头痛，我尿酸这几年怎么样"}}, day,
		func(s Step) { heard = append(heard, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.bodies) != 2 || f.auth[0] != "Bearer sk-secret" {
		t.Fatalf("requests %d auth %q", len(f.bodies), f.auth)
	}
	first := f.bodies[0]
	for _, want := range []string{`"name":"health_overview"`, `"name":"get_lab_trend"`, `"name":"add_record"`, `"name":"update_record"`,
		"先调用 health_overview", "名册", "胃炎", "今天头痛", `"tool_choice":"auto"`} {
		if !strings.Contains(first, want) {
			t.Errorf("first request has no %s", want)
		}
	}
	for _, not := range []string{`"name":"delete_record"`, `"name":"add_attachment"`, `"name":"undo_checkin"`, "近期档案", "response_format"} {
		if strings.Contains(first, not) {
			t.Errorf("first request has %s", not)
		}
	}
	second := f.bodies[1]
	// the trend came from the archive, all of it; the assistant's message
	// went back as it was sent
	for _, want := range []string{`"role":"tool"`, `"tool_call_id":"call_a"`, "2019-03-02", "已整理成第 1 条", "reasoning_content"} {
		if !strings.Contains(second, want) {
			t.Errorf("second request has no %s", want)
		}
	}
	if !strings.Contains(d.Reply, "430") || len(d.Items) != 1 || d.Items[0].Type != "symptoms" {
		t.Fatalf("draft %+v", d)
	}
	if len(d.Steps) != 2 || d.Steps[0].Tool != "get_lab_trend" || d.Steps[0].Text != "尿酸" || d.Steps[1].Type != "symptoms" || len(heard) != 2 {
		t.Fatalf("steps %+v heard %+v", d.Steps, heard)
	}
	if d.Usage.TotalTokens != 250 || d.Usage.CachedTokens != 80 {
		t.Fatalf("usage %+v", d.Usage)
	}
	if _, n, _ := st.List("symptoms", store.ListQuery{Limit: 5}); n != 0 {
		t.Fatal("a draft was written")
	}
}

func TestToolErrorGoesBackToModel(t *testing.T) {
	st := openStore(t)
	f := &fakeModel{t: t, replies: []string{
		calls("add_record", `{"type":"visits","fields":{"visit_date":"2026-10-27","visit_type":"很高"}}`, "delete_record", `{"type":"visits","id":1}`, "list_records", `not json`),
		calls("add_record", `{"type":"visits","fields":{"visit_date":"2026-10-27","visit_type":"门诊"}}`),
		answer("记下了今天的门诊。"),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "今天去看了门诊"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.bodies) != 3 || !strings.Contains(f.bodies[1], "不在可选值中") || !strings.Contains(f.bodies[1], "这里没有 delete_record") || !strings.Contains(f.bodies[1], "参数不是 JSON 对象") {
		t.Fatalf("errors did not go back: %d\n%s", len(f.bodies), f.bodies[1])
	}
	if len(d.Items) != 1 || d.Items[0].Fields["visit_type"] != "门诊" {
		t.Fatalf("%+v", d.Items)
	}
	if !d.Steps[0].Error || !d.Steps[1].Error || !d.Steps[2].Error || d.Steps[3].Error {
		t.Fatalf("steps %+v", d.Steps)
	}
}

// The write tools are never run against the store from the assistant, even
// if one were marked read-only by mistake.
func TestWriteToolsNeverRun(t *testing.T) {
	st := openStore(t)
	tb, err := openTools(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.close()
	for name := range draftTools {
		if tb.read[name] {
			t.Errorf("%s is on the read list", name)
		}
	}
	out, failed := tb.run(context.Background(), "add_record", map[string]any{"type": "symptoms", "fields": map[string]any{"occurred_at": "2026-10-27 09:00", "description": "头痛"}})
	if !failed || !strings.Contains(out, "不是只读") {
		t.Fatalf("%v %s", failed, out)
	}
	if _, n, _ := st.List("symptoms", store.ListQuery{Limit: 5}); n != 0 {
		t.Fatal("written")
	}
	if !tb.read["health_overview"] || !tb.read["get_lab_trend"] {
		t.Fatal("read tools missing")
	}
}

// Only putting cards together, all fine, ends the reply: the model is not
// asked again just to say so.
func TestCardsOnlyRoundEndsTheReply(t *testing.T) {
	st := openStore(t)
	symptom := `{"type":"symptoms","fields":{"occurred_at":"2026-10-27 09:00","description":"头痛","severity":3}}`
	for _, c := range []struct{ said, reply string }{{"记下了今天早上的头痛。", "记下了今天早上的头痛。"}, {"", ""}} {
		f := &fakeModel{t: t, replies: []string{callsSaying(c.said, "add_record", symptom), answer("不该有这一次")}}
		d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "今天早上头痛"}}, day, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.bodies) != 1 || d.Reply != c.reply || len(d.Items) != 1 || d.Unclear != "" || d.Usage.TotalTokens != 110 {
			t.Fatalf("%q: requests %d draft %+v", c.said, len(f.bodies), d)
		}
	}
	// something was looked up and no answer written yet: ask once more
	f := &fakeModel{t: t, replies: []string{calls("health_overview", `{}`), calls("add_record", symptom), answer("尿酸正常，头痛记下了。")}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "我最近怎么样，今天头痛"}}, day, nil)
	if err != nil || len(f.bodies) != 3 || d.Reply != "尿酸正常，头痛记下了。" {
		t.Fatalf("requests %d err %v draft %+v", len(f.bodies), err, d)
	}
}

func TestClaimWithoutCardIsSentBack(t *testing.T) {
	st := openStore(t)
	appt := `{"type":"appointments","fields":{"title":"复查","scheduled_at":"2026-12-15 09:00","appt_type":"检查"}}`
	ask := []Turn{{Role: "user", Text: "可以"}}

	// says it booked, makes no card: told once, then makes the card
	f := &fakeModel{t: t, replies: []string{answer("这个预约已经创建好了，12月14日晚上提醒您。"), callsSaying("整理好了，保存后生效。", "add_record", appt)}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), ask, day, nil)
	if err != nil || len(f.bodies) != 2 || len(d.Items) != 1 || d.Reply != "整理好了，保存后生效。" {
		t.Fatalf("requests %d err %v draft %+v", len(f.bodies), err, d)
	}
	if !strings.Contains(f.bodies[1], "App 检查") {
		t.Fatal("second request does not say why")
	}

	// still no card: the reply says nothing was saved
	f = &fakeModel{t: t, replies: []string{answer("已为您设置提醒。")}}
	d, err = Propose(context.Background(), st, f.cfg(f.start()), ask, day, nil)
	if err != nil || len(f.bodies) != 2 || len(d.Items) != 0 || !strings.HasSuffix(d.Reply, claimNote) {
		t.Fatalf("requests %d err %v draft %+v", len(f.bodies), err, d)
	}

	// an offer or a question is not a claim
	f = &fakeModel{t: t, replies: []string{answer("我来帮您预约，几点方便？")}}
	d, _ = Propose(context.Background(), st, f.cfg(f.start()), ask, day, nil)
	if len(f.bodies) != 1 || strings.Contains(d.Reply, claimNote) {
		t.Fatalf("requests %d draft %+v", len(f.bodies), d)
	}
}

func TestClaimsDone(t *testing.T) {
	for s, want := range map[string]bool{
		"这个预约已经创建好了":      true,
		"我已经为您安排了以下预约":    true,
		"已为您设置每天早上9点提醒":   true,
		"提醒设好了":           true,
		"这条已经记录过了，不用再记":   false,
		"我来帮您预约检查":        false,
		"整理好了，保存后生效":      false,
		"建议1-2个月后复查，要预约吗": false,
	} {
		if got := claimsDone(s); got != want {
			t.Errorf("%q: %v", s, got)
		}
	}
}

func TestReplyLosesMarkdown(t *testing.T) {
	got := plainReply("## 复查\n**检查日期**：12月15日\n- 血尿酸\n  * 肾功能\n1-2 个月")
	if want := "复查\n检查日期：12月15日\n· 血尿酸\n  · 肾功能\n1-2 个月"; got != want {
		t.Fatalf("%q", got)
	}
}

func TestFieldTableSentOnce(t *testing.T) {
	st := openStore(t)
	tb, err := openTools(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.close()
	b, _ := json.Marshal(tb.defs)
	if n := strings.Count(string(b), fieldTable); n != 1 {
		t.Fatalf("field table %d times", n)
	}
	if !strings.Contains(string(b), "见 add_record 的说明") {
		t.Fatal("list_records does not point to add_record")
	}
	t.Logf("tool definitions: %d characters", utf8.RuneCount(b))
}

func TestStopsAfterMaxRounds(t *testing.T) {
	st := openStore(t)
	f := &fakeModel{t: t, replies: []string{calls("health_overview", `{}`)}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "看看"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.bodies) != maxRounds || !strings.Contains(f.bodies[maxRounds-1], `"tool_choice":"none"`) {
		t.Fatalf("requests %d", len(f.bodies))
	}
	if d.Unclear == "" || len(d.Steps) != maxRounds-1 {
		t.Fatalf("%+v", d)
	}
}

func TestMedicineQuestionKeepsDiagnosisOut(t *testing.T) {
	st := openStore(t)
	cond, _ := st.Create("conditions", store.Record{"name": "高尿酸血症", "status": "观察中"})
	if _, err := st.Create("medications", store.Record{"name": "非布司他", "status": "计划中"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeModel{t: t, replies: []string{
		calls("add_record", `{"type":"conditions","fields":{"name":"痛风","status":"观察中"}}`,
			"add_record", `{"type":"medications","fields":{"name":"非布司他"}}`,
			"add_record", `{"type":"medications","fields":{"name":"苯溴马隆","condition_name":"痛风"}}`,
			"add_record", `{"type":"medications","fields":{"name":"别嘌醇","purpose":"降尿酸"}}`),
		answer("可以考虑别嘌醇，用药前要医生确认。"),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "给我推荐药品"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 1 || d.Items[0].Fields["name"] != "别嘌醇" || d.Items[0].Fields["status"] != "计划中" {
		t.Fatalf("items %+v", d.Items)
	}
	for _, want := range []string{"诊断只写在回复里", "medication_id", "没有叫「痛风」的健康问题"} {
		if !strings.Contains(f.bodies[1], want) {
			t.Errorf("model was not told %q", want)
		}
	}

	w := newDrafting(st, []Turn{{Role: "user", Text: "我现在有什么问题"}}, day)
	if msg, bad := w.draft("add_record", map[string]any{"type": "conditions", "fields": map[string]any{"name": "高尿酸血症"}}); !bad || !strings.Contains(msg, "#"+jsonNum(cond["id"])) {
		t.Fatalf("duplicate on file: %s", msg)
	}
	if msg, bad := w.draft("add_record", map[string]any{"type": "conditions", "fields": map[string]any{"name": "痛风", "status": "观察中"}}); bad {
		t.Fatal(msg)
	}
	if w.items[0].Fields["onset_date"] != "2026-10-27" {
		t.Fatalf("onset %+v", w.items[0].Fields)
	}
	if msg, bad := w.draft("add_record", map[string]any{"type": "conditions", "fields": map[string]any{"name": "痛风"}}); !bad || !strings.Contains(msg, "这次已经整理过") {
		t.Fatalf("twice in one reply: %s", msg)
	}
	// a medicine for the issue drafted just now may name it
	if msg, bad := w.draft("add_record", map[string]any{"type": "medications", "fields": map[string]any{"name": "秋水仙碱", "condition_name": "痛风"}}); bad {
		t.Fatal(msg)
	}
	// a metric kind made in this reply makes its reading valid
	if msg, bad := w.draft("add_record", map[string]any{"type": "metric_kinds", "fields": map[string]any{"name": "握力", "unit": "kg"}}); bad {
		t.Fatal(msg)
	}
	if msg, bad := w.draft("add_record", map[string]any{"type": "metrics", "fields": map[string]any{"metric_type": "握力", "value": 32, "measured_at": "2026-10-27 08:00"}}); bad {
		t.Fatal(msg)
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestUpdateBecomesCard(t *testing.T) {
	st := openStore(t)
	med, err := st.Create("medications", store.Record{"name": "多替诺雷", "status": "计划中"})
	if err != nil {
		t.Fatal(err)
	}
	id := idOf(med)
	f := &fakeModel{t: t, replies: []string{
		calls("update_record", `{"type":"medications","id":`+jsonNum(id)+`,"fields":{"status":"很好"}}`,
			"update_record", `{"type":"medications","id":999,"fields":{"status":"在用"}}`,
			"update_record", `{"type":"medications","id":`+jsonNum(id)+`,"fields":{"remind_times":"09:00","status":"在用"}}`),
		answer("改好了提醒时间，点保存后生效。"),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "提醒我每天早上9点吃多替诺雷"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 1 || d.Items[0].Kind != "update" || d.Items[0].ID != id || d.Items[0].Before["status"] != "计划中" || d.Items[0].Summary != "多替诺雷" {
		t.Fatalf("items %+v", d.Items)
	}
	if !strings.Contains(f.bodies[1], "不在可选值中") || !strings.Contains(f.bodies[1], "没有 medications #999") || !strings.Contains(f.bodies[1], "现在还没改") {
		t.Fatalf("second request:\n%s", f.bodies[1])
	}
	if got, _ := st.Get("medications", id); got["status"] != "计划中" {
		t.Fatal("changed before saving")
	}
	res := Commit(st, d.Items)
	if !res.OK || len(res.Saved) != 1 || res.Saved[0].Kind != "update" || res.Saved[0].ID != id {
		t.Fatalf("%+v", res)
	}
	got, _ := st.Get("medications", id)
	if got["status"] != "在用" || got["remind_times"] != "09:00" {
		t.Fatalf("%+v", got)
	}
}

func TestErrorsHideTheKey(t *testing.T) {
	st := openStore(t)
	for _, c := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusUnauthorized, `{"error":"bad sk-secret"}`, "401"},
		{http.StatusBadRequest, `{"error":{"message":"tools is not supported sk-secret"}}`, "不支持工具调用"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			io.WriteString(w, c.body)
		}))
		_, err := Propose(context.Background(), st, Config{BaseURL: srv.URL, APIKey: "sk-secret", Model: "m"}, []Turn{{Role: "user", Text: "头痛"}}, day, nil)
		srv.Close()
		if err == nil || strings.Contains(err.Error(), "sk-secret") || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%d: %v", c.status, err)
		}
	}
}

func TestCommitRollsBackAndPartial(t *testing.T) {
	st := openStore(t)
	res := Commit(st, []Item{
		{Kind: "record", Type: "conditions", Fields: map[string]any{"name": "胃炎"}},
		{Kind: "record", Type: "visits", Fields: map[string]any{"visit_date": "2026-10-27", "visit_type": "很高"}},
	})
	if res.OK || res.Error == "" {
		t.Fatalf("%+v", res)
	}
	if _, n, _ := st.List("conditions", store.ListQuery{Limit: 10}); n != 0 {
		t.Fatalf("kept condition n=%d", n)
	}

	res = Commit(st, []Item{
		{Kind: "record", Type: "symptoms", Fields: map[string]any{"occurred_at": "2026-10-27 09:00", "description": "头痛", "severity": 4}},
		{Kind: "checkin", Habit: "没有这项"},
	})
	if res.OK || len(res.Saved) != 1 || res.FailedIndex == nil || *res.FailedIndex != 1 {
		t.Fatalf("%+v", res)
	}
	if _, n, _ := st.List("symptoms", store.ListQuery{Limit: 10}); n != 1 {
		t.Fatalf("symptom n=%d", n)
	}

	if _, err := st.Create("habits", store.Record{"name": "晨跑"}); err != nil {
		t.Fatal(err)
	}
	res = Commit(st, []Item{
		{Kind: "record", Type: "conditions", Fields: map[string]any{"name": "胃炎"}},
		{Kind: "record", Type: "symptoms", Fields: map[string]any{"occurred_at": "2026-10-27 10:00", "description": "隐痛"}, ConditionName: "胃炎"},
		{Kind: "checkin", Habit: "晨跑", Date: time.Now().Format("2006-01-02")},
		{Kind: "lab_report", Report: map[string]any{"report_date": "2026-10-27", "title": "血常规", "report_type": "化验"},
			Results: []LabResult{{ItemName: "白细胞", Value: "11.2", Unit: "10^9/L", RefRange: "3.5-9.5"}}},
	})
	if !res.OK || len(res.Saved) != 4 {
		t.Fatalf("%+v", res)
	}
	sym, _, _ := st.List("symptoms", store.ListQuery{Q: "隐痛", Limit: 5})
	cond, _, _ := st.List("conditions", store.ListQuery{Q: "胃炎", Limit: 5})
	res = Commit(st, []Item{{Kind: "widget"}})
	if res.OK || !strings.Contains(res.Error, "不能记这种类型") {
		t.Fatalf("unknown kind %+v", res)
	}
	res = Commit(st, []Item{{Kind: "checkin", Habit: "晨跑", Date: "2020-01-01"}})
	if res.OK || !strings.Contains(res.Error, "只能记在今天") {
		t.Fatalf("past check-in %+v", res)
	}

	if len(sym) != 1 || len(cond) != 1 || sym[0]["condition_id"] != cond[0]["id"] {
		b, _ := json.Marshal(sym)
		t.Fatalf("link %s", b)
	}
}

func TestProposeSendsImageAndRejectsOtherFiles(t *testing.T) {
	if _, err := cleanImages([]Image{{MIME: "image/svg+xml", Data: "PHN2Zy8+"}}); err == nil {
		t.Fatal("svg was accepted")
	}
	f := &fakeModel{t: t, replies: []string{answer("这张看不清，请再拍一张。")}}
	st := openStore(t)
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{
		Role: "user", Text: "这是化验单", Images: []Image{{MIME: "image/jpeg", Data: jpg}},
	}}, day, nil)
	if err != nil || !strings.Contains(f.bodies[0], "image_url") || !strings.Contains(f.bodies[0], "data:image/jpeg;base64,") {
		t.Fatalf("err=%v", err)
	}
	if d.Reply == "" || d.Unclear != "" || len(d.Items) != 0 || d.Usage.TotalTokens != 140 {
		t.Fatalf("%+v", d)
	}
}

// a 1x1 jpeg
const jpg = "/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////wgALCAABAAEBAREA/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA="

func TestExchangeLog(t *testing.T) {
	st := openStore(t)
	f := &fakeModel{t: t, replies: []string{
		calls("search_records", `{"query":"头痛"}`),
		answer("日志测试的回答"),
	}}
	cfg := f.cfg(f.start())
	cfg.APIKey, cfg.Model = "sk-log-secret-should-not-appear", "glm-4.6v"
	if _, err := Propose(context.Background(), st, cfg, []Turn{{
		Role: "user", Text: "帮我记下头痛", Images: []Image{{MIME: "image/jpeg", Data: jpg}},
	}}, day, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(st.Dir, "assistant.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// what was asked, each request, the call with its arguments, the
	// answer: enough to see where the tokens went
	for _, want := range []string{"glm-4.6v", "帮我记下头痛", "（图片 image/jpeg", "第 1 次请求", "第 2 次请求",
		`调用 search_records {"query":"头痛"}`, "日志测试的回答", "2 次请求", "工具说明"} {
		if !strings.Contains(s, want) {
			t.Errorf("log missing %q\n%s", want, s)
		}
	}
	// the key, the picture and the system prompt stay out
	for _, not := range []string{cfg.APIKey, jpg[:40], "名册"} {
		if strings.Contains(s, not) {
			t.Fatalf("log has %q:\n%s", not, s)
		}
	}
	fi, _ := os.Stat(filepath.Join(st.Dir, "assistant.log"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
}

func TestLooseAppointmentShape(t *testing.T) {
	it := itemFromMap(map[string]any{"kind": "record", "type": "appointment", "scheduled_at": "2026-10-28 14:00", "title": "口腔科的治疗", "appt_type": "门诊", "place": "口腔科"})
	if it.Kind != "record" || it.Type != "appointments" {
		t.Fatalf("%+v", it)
	}
	if it.Fields["title"] != "口腔科的治疗" || it.Fields["scheduled_at"] != "2026-10-28 14:00" {
		t.Fatalf("fields %+v", it.Fields)
	}
	st := openStore(t)
	if p := problem(st, it, time.Date(2026, 10, 27, 15, 0, 0, 0, time.Local)); p != "" {
		t.Fatal(p)
	}
}

func TestInstructionsKeepTheLimitsAndCacheOrder(t *testing.T) {
	st := openStore(t)
	if _, err := st.Create("medications", store.Record{"name": "非布司他", "status": "在用"}); err != nil {
		t.Fatal(err)
	}
	s := Instructions(st, "MCP 的说明", day)
	if !strings.HasPrefix(s, "MCP 的说明") {
		t.Fatal("the MCP instructions should come first")
	}
	for _, want := range []string{"结果仅供参考", "需要去医院", "观察中", "计划中", "update_record", "不要说「已设置」", "非布司他"} {
		if !strings.Contains(s, want) {
			t.Errorf("instructions have no %s", want)
		}
	}
	// what changes every minute is last, after what changes with the archive
	if !(strings.Index(s, inApp) < strings.Index(s, "名册") && strings.Index(s, "名册") < strings.Index(s, "现在大约 09:30")) {
		t.Fatal("order")
	}
}

func TestUpdateRequiresKeyWhenHostChanges(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base, key, model := "https://open.bigmodel.cn/api/paas/v4", "sk-old", "m"
	cfg, err = cfg.Update(&base, &key, &model, false)
	if err != nil {
		t.Fatal(err)
	}
	same := "https://open.bigmodel.cn/api/paas/v3"
	kept, err := cfg.Update(&same, nil, nil, false)
	if err != nil || kept.APIKey != "sk-old" {
		t.Fatalf("same host %+v %v", kept, err)
	}
	other := "https://api.openai.com/v1"
	if _, err := cfg.Update(&other, nil, nil, false); err == nil || !strings.Contains(err.Error(), "重新填写密钥") {
		t.Fatal(err)
	}
	blank := ""
	if _, err := cfg.Update(&other, &blank, nil, false); err == nil {
		t.Fatal("a blank key went to another host")
	}
	// the same key typed again for the new host (a gateway that takes it)
	moved, err := cfg.Update(&other, &key, nil, false)
	if err != nil || moved.BaseURL != other || moved.APIKey != "sk-old" {
		t.Fatalf("same key typed again %+v %v", moved, err)
	}
	// clearing the address leaves the key with no host to go to
	cleared, err := cfg.Update(&blank, nil, nil, false)
	if err != nil || cleared.BaseURL != "" {
		t.Fatalf("clear address %+v %v", cleared, err)
	}
	// and an address filled in after that still needs the key again
	if _, err := cleared.Update(&other, nil, nil, false); err == nil {
		t.Fatal("the key went to a host filled in after clearing the address")
	}
	// clearing the key while moving is fine too
	if _, err := cfg.Update(&other, nil, nil, true); err != nil {
		t.Fatal(err)
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
