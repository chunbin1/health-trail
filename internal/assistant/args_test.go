package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/lee/health-trail/internal/store"
)

// glm-4.6v, 2026-10-28: the arguments of add_record as the service sent
// them. Only "type" was JSON; fields was left in its value as markup.
const glmLeak = `{"type": "habits\n<arg_key>fields</arg_key>\n<arg_value>{\"name\": \"喝水\", \"unit\": \"升\", \"target\": 3}"}`

func TestArgsOf(t *testing.T) {
	habit := map[string]any{"type": "habits", "fields": map[string]any{"name": "喝水", "unit": "升", "target": 3.0}}
	for _, c := range []struct {
		name, raw string
		want      map[string]any
	}{
		{"glm leak", glmLeak, habit},
		{"leak with closing tags", `{"type": "habits</arg_value><arg_key>fields</arg_key><arg_value>{\"name\": \"喝水\", \"unit\": \"升\", \"target\": 3}</arg_value>"}`, habit},
		{"markup only", "<arg_key>type</arg_key><arg_value>habits</arg_value>\n<arg_key>fields</arg_key><arg_value>{\"name\": \"喝水\", \"unit\": \"升\", \"target\": 3}</arg_value>", habit},
		{"json wins over the leak", `{"type": "habits", "fields": {"name": "喝水", "unit": "升", "target": 3}, "notes": "x<arg_key>type</arg_key><arg_value>metrics"}`,
			map[string]any{"type": "habits", "fields": map[string]any{"name": "喝水", "unit": "升", "target": 3.0}, "notes": "x"}},
		{"plain json", `{"type":"habits","fields":{"name":"喝水","unit":"升","target":3}}`, habit},
		{"angle brackets that are not markup", `{"query":"<5 天"}`, map[string]any{"query": "<5 天"}},
		{"none", "", map[string]any{}},
	} {
		got, err := argsOf(c.raw)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %#v %v", c.name, got, err)
		}
	}
	for _, raw := range []string{"not json", `["type"]`, "<arg_key></arg_key>"} {
		if _, err := argsOf(raw); !errors.Is(err, errBadArgs) {
			t.Errorf("%q: %v", raw, err)
		}
	}
	name, args, err := callOf("checkin<arg_key>habit</arg_key><arg_value>喝水</arg_value>", `{"value": 3}`)
	if err != nil || name != "checkin" || !reflect.DeepEqual(args, map[string]any{"habit": "喝水", "value": 3.0}) {
		t.Fatalf("name markup: %q %#v %v", name, args, err)
	}
}

// What the window showed: "整理一条habits <arg_key>fields</arg_key> …".
// A step never carries markup, or a type that isn't one.
func TestStepHasNoMarkup(t *testing.T) {
	s := stepOf("add_record", map[string]any{"type": "habits\n<arg_key>fields</arg_key>\n<arg_value>{}"})
	if s.Type != "" {
		t.Fatalf("type %q", s.Type)
	}
	s = stepOf("checkin", map[string]any{"habit": "喝水</arg_value>\n<arg_key>value"})
	if s.Text != "喝水 value" {
		t.Fatalf("text %q", s.Text)
	}
	if s := stepOf("add_record", map[string]any{"type": "habits"}); s.Type != "habits" {
		t.Fatalf("plain type lost: %+v", s)
	}
}

// The case from the window: "打卡今天喝3L水" with no such habit. The leaked
// add_record is read as a new habit, and the check-in on it in the same
// message is a card too; saving writes both.
func TestLeakedHabitAndCheckin(t *testing.T) {
	st := openStore(t)
	f := &fakeModel{t: t, replies: []string{
		callsSaying("新建喝水打卡项并打卡。", "add_record", glmLeak, "checkin", `{"habit": "喝水", "value": 3}`),
		answer("不该有这一次"),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "打卡今天喝3L水"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.bodies) != 1 || len(d.Items) != 2 || d.Items[0].Type != "habits" || d.Items[1].Kind != "checkin" {
		t.Fatalf("requests %d items %+v", len(f.bodies), d.Items)
	}
	if d.Steps[0].Type != "habits" || d.Steps[0].Error || d.Steps[1].Error {
		t.Fatalf("steps %+v", d.Steps)
	}
	if res := Commit(st, d.Items, "zh"); !res.OK {
		t.Fatalf("%+v", res)
	}
	h, err := st.FindHabit("喝水")
	if err != nil || h["unit"] != "升" {
		t.Fatalf("%v %v", h, err)
	}
	if _, n, _ := st.List("checkins", store.ListQuery{Limit: 5}); n != 1 {
		t.Fatalf("checkins %d", n)
	}
}

// A check-in on a habit that isn't there and isn't drafted still fails,
// and says how to add one.
func TestCheckinWithoutHabit(t *testing.T) {
	st := openStore(t)
	f := &fakeModel{t: t, replies: []string{
		calls("checkin", `{"habit": "喝水", "value": 3}`),
		calls("add_record", `{"type":"habits","fields":{"name":"喝水","unit":"升"}}`, "checkin", `{"habit": "喝水", "value": 3}`),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "打卡今天喝3L水"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.bodies[1], "add_record 写一条 habits") || len(d.Items) != 2 || len(f.bodies) != 2 {
		t.Fatalf("requests %d items %+v\n%s", len(f.bodies), d.Items, f.bodies[1])
	}
}

// "打开喝水": an archived habit is in the roster with its id; a check-in
// on it says to restore it, and restoring it in the same reply makes the
// check-in a card. Saving restores it before checking in.
func TestArchivedHabitIsRestored(t *testing.T) {
	st := openStore(t)
	h, err := st.Create("habits", store.Record{"name": "喝水", "unit": "升", "archived": true})
	if err != nil {
		t.Fatal(err)
	}
	id := idOf(h)
	f := &fakeModel{t: t, replies: []string{
		calls("checkin", `{"habit": "喝水"}`),
		callsSaying("恢复喝水，并打卡。", "update_record", `{"type":"habits","id":`+jsonNum(id)+`,"fields":{"archived":false}}`, "checkin", `{"habit": "喝水", "value": 2}`),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "帮我打开喝水，今天喝了2升"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.bodies[0], `已归档的打卡项：\n- id `+jsonNum(id)+" 喝水") {
		t.Fatal("archived habit missing from the roster")
	}
	if !strings.Contains(f.bodies[1], "已归档") || !strings.Contains(f.bodies[1], "archived 改成 false") {
		t.Fatalf("second request:\n%s", f.bodies[1])
	}
	if len(d.Items) != 2 || d.Items[0].Kind != "update" || d.Items[1].Kind != "checkin" {
		t.Fatalf("items %+v", d.Items)
	}
	// through JSON, as the window sends it back
	var items []Item
	b, _ := json.Marshal(d.Items)
	json.Unmarshal(b, &items)
	if res := Commit(st, items, "zh"); !res.OK {
		t.Fatalf("%+v", res)
	}
	got, _ := st.Get("habits", id)
	if truthy(got["archived"]) {
		t.Fatal("still archived")
	}
	if _, n, _ := st.List("checkins", store.ListQuery{Limit: 5}); n != 1 {
		t.Fatalf("checkins %d", n)
	}
}

// The same call failing the same way in the next round ends the reply
// with a sentence for the person, instead of using up the rounds.
func TestSameFailureTwiceStops(t *testing.T) {
	for _, c := range []struct {
		name, lang, args, want string
	}{
		{"no habit", "zh", `{"habit": "喝水"}`, "没有叫「喝水」的打卡项"},
		{"unreadable", "zh", `not json`, "参数格式不对"},
		{"english", "en", `{"habit": "喝水"}`, "failed twice"},
	} {
		st := openStore(t)
		f := &fakeModel{t: t, replies: []string{callsSaying("我来打卡。", "checkin", c.args)}}
		cfg := f.cfg(f.start())
		cfg.Lang = c.lang
		d, err := Propose(context.Background(), st, cfg, []Turn{{Role: "user", Text: "打卡喝水"}}, day, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.bodies) != 2 || !strings.Contains(d.Reply, c.want) || strings.Contains(d.Reply, "我来打卡") || d.Unclear != "" {
			t.Errorf("%s: requests %d reply %q", c.name, len(f.bodies), d.Reply)
		}
		if strings.Contains(d.Reply, "<arg_key>") || strings.Contains(d.Reply, "JSON") {
			t.Errorf("%s: reply %q", c.name, d.Reply)
		}
	}
}

// A failed call the model then fixes is not stopped, even with the same
// tool, nor is a different failure in the next round.
func TestFixedCallIsNotStopped(t *testing.T) {
	st := openStore(t)
	f := &fakeModel{t: t, replies: []string{
		calls("add_record", `not json`),
		calls("add_record", `{"type":"visits","fields":{"visit_date":"2026-10-27","visit_type":"很高"}}`),
		calls("add_record", `{"type":"visits","fields":{"visit_date":"2026-10-27","visit_type":"门诊"}}`),
		answer("不该有这一次"),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "今天去看了门诊"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.bodies) != 3 || len(d.Items) != 1 || strings.Contains(d.Reply, "停下") {
		t.Fatalf("requests %d items %+v reply %q", len(f.bodies), d.Items, d.Reply)
	}
}

// Seen from glm-4.6v: the check-in failed, the habit card went through,
// and the reply ended there without the check-in. A tool that failed and
// hasn't worked since gets the model asked once more.
func TestFailedCheckinGetsAnotherRound(t *testing.T) {
	st := openStore(t)
	f := &fakeModel{t: t, replies: []string{
		calls("checkin", `{"habit": "喝水", "value": 3}`),
		calls("add_record", `{"type":"habits","fields":{"name":"喝水","unit":"L"}}`),
		callsSaying("建好了喝水，也打了卡，保存后生效。", "checkin", `{"habit": "喝水", "value": 3}`),
		answer("不该有这一次"),
	}}
	d, err := Propose(context.Background(), st, f.cfg(f.start()), []Turn{{Role: "user", Text: "打卡今天喝3L水"}}, day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.bodies) != 3 || len(d.Items) != 2 || d.Items[1].Kind != "checkin" {
		t.Fatalf("requests %d items %+v", len(f.bodies), d.Items)
	}
}
