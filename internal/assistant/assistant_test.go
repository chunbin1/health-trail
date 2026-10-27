package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

func TestDraftAndRedact(t *testing.T) {
	st := openStore(t)
	if _, err := st.Create("conditions", store.Record{"name": "胃炎", "status": "进行中"}); err != nil {
		t.Fatal(err)
	}
	var sawKey, sawText, sawFormat bool
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "Bearer sk-secret" {
			sawKey = true
		}
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "今天头痛") {
			sawText = true
		}
		if strings.Contains(string(b), "胃炎") && strings.Contains(string(b), "response_format") {
			sawFormat = true
		}
		if calls == 1 && strings.Contains(string(b), "response_format") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"response_format unsupported sk-secret"}}`))
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"items\":[{\"kind\":\"record\",\"type\":\"symptoms\",\"fields\":{\"occurred_at\":\"2026-10-27 09:00\",\"description\":\"头痛\",\"severity\":4}}],\"unclear\":\"\"}"}}]}`)
	}))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL + "/v1", APIKey: "sk-secret", Model: "m"}
	d, err := Propose(context.Background(), st, cfg, "今天头痛", time.Date(2026, 10, 27, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if !sawKey || !sawText || !sawFormat || calls != 2 {
		t.Fatalf("sawKey=%v sawText=%v sawFormat=%v calls=%d", sawKey, sawText, sawFormat, calls)
	}
	if len(d.Items) != 1 || d.Items[0].Type != "symptoms" || d.Items[0].Problem != "" {
		t.Fatalf("%+v", d)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad sk-secret"}`))
	}))
	defer bad.Close()
	cfg.BaseURL = bad.URL
	_, err = Propose(context.Background(), st, cfg, "今天头痛", time.Now())
	if err == nil || strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("error should hide the key: %v", err)
	}
}

func TestParseFencedAndBad(t *testing.T) {
	d, err := parseDraft("```json\n{\"items\":[{\"type\":\"metrics\",\"fields\":{\"metric_type\":\"体重\",\"value\":70,\"measured_at\":\"2026-10-27 08:00\"}}],\"unclear\":\"剂量没听清\"}\n```")
	if err != nil || len(d.Items) != 1 || d.Items[0].Kind != "record" || d.Unclear == "" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := parseDraft("not json"); err == nil {
		t.Fatal("expected error")
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
	if len(sym) != 1 || len(cond) != 1 || sym[0]["condition_id"] != cond[0]["id"] {
		b, _ := json.Marshal(sym)
		t.Fatalf("link %s", b)
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
