package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lee/health-trail/internal/store"
)

func session(t *testing.T) (*mcp.ClientSession, *store.Store) {
	t.Helper()
	return sessionFor(t, true)
}

// sessionFor connects a client on this computer (local) or another device.
func sessionFor(t *testing.T, local bool) (*mcp.ClientSession, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	ct, svt := mcp.NewInMemoryTransports()
	ss, err := New(st, "test", local).Connect(ctx, svt, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, st
}

// call runs a tool and returns its text and whether it reported an error.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestToolsListed(t *testing.T) {
	cs, _ := session(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 15 {
		t.Errorf("%d tools", len(res.Tools))
	}
	for _, tool := range res.Tools {
		if tool.Name == "add_record" && !strings.Contains(tool.Description, "visit_date 就诊日期（必填，日期）") {
			t.Errorf("add_record doesn't describe the schema: %s", tool.Description)
		}
	}
}

// What Claude does with a screenshot of a report: one add_lab_report call.
func TestAddLabReportFromScreenshot(t *testing.T) {
	cs, st := session(t)
	out, isErr := call(t, cs, "add_lab_report", map[string]any{
		"report": map[string]any{"report_date": "2026-10-15", "title": "血常规+CRP", "report_type": "化验"},
		"panel":  "血常规",
		"results": []any{
			map[string]any{"item_name": "超敏C-反应蛋白", "value": "111.87↑", "unit": "mg/L", "ref_range": "0-6", "panel": "炎症指标"},
			map[string]any{"item_name": "白细胞", "value": "17.62 ↑", "unit": "10^9/L", "ref_range": "3.5-9.5"},
			map[string]any{"item_name": "淋巴细胞比例", "value": 14.1, "unit": "%", "ref_range": "20-50"},
			map[string]any{"item_name": "血红蛋白", "value": "153", "unit": "g/L", "ref_range": "130-175"},
		},
	})
	if isErr {
		t.Fatal(out)
	}
	var res struct {
		Saved    int      `json:"saved"`
		Abnormal int      `json:"abnormal_count"`
		Results  []string `json:"results"`
	}
	json.Unmarshal([]byte(out), &res)
	if res.Saved != 4 || res.Abnormal != 3 {
		t.Errorf("saved %d abnormal %d: %s", res.Saved, res.Abnormal, out)
	}
	rows, _, _ := st.List("lab_results", store.ListQuery{Filters: map[string]any{"panel": "炎症指标"}})
	if len(rows) != 1 || rows[0]["value_num"] != 111.87 || rows[0]["flag"] != "偏高" {
		t.Errorf("CRP row = %v", rows)
	}

	// a bad row stops the whole report, report included
	out, isErr = call(t, cs, "add_lab_report", map[string]any{
		"report":  map[string]any{"report_date": "2026-10-16", "title": "复查"},
		"results": []any{map[string]any{"item_name": "白细胞", "value": "9"}, map[string]any{"item_name": "血小板", "value": "1", "flag": "很高"}},
	})
	if !isErr || !strings.Contains(out, "第 2 条") {
		t.Errorf("bad row: %v %s", isErr, out)
	}
	if _, n, _ := st.List("reports", store.ListQuery{}); n != 1 {
		t.Errorf("%d reports after a failed import, want 1", n)
	}

	out, _ = call(t, cs, "get_lab_trend", map[string]any{"item_name": "反应蛋白"})
	if !strings.Contains(out, "111.87") {
		t.Errorf("trend: %s", out)
	}
}

func TestErrorsReachClaude(t *testing.T) {
	cs, _ := session(t)
	out, isErr := call(t, cs, "add_record", map[string]any{"type": "conditions", "fields": map[string]any{"name": "胃炎", "status": "好了"}})
	if !isErr || !strings.Contains(out, "已痊愈") {
		t.Errorf("bad enum: %v %s", isErr, out)
	}
	out, isErr = call(t, cs, "add_record", map[string]any{"type": "illnesses", "fields": map[string]any{}})
	if !isErr || !strings.Contains(out, "conditions") {
		t.Errorf("bad type: %v %s", isErr, out)
	}
}

func TestCheckinFlow(t *testing.T) {
	cs, _ := session(t)
	out, isErr := call(t, cs, "checkin", map[string]any{"habit": "跑步", "value": 5})
	if !isErr || !strings.Contains(out, "create_if_missing") {
		t.Errorf("missing habit: %s", out)
	}
	if out, isErr = call(t, cs, "checkin", map[string]any{"habit": "跑步", "value": 5, "create_if_missing": true, "unit": "公里"}); isErr {
		t.Fatal(out)
	}
	out, _ = call(t, cs, "get_checkin_stats", map[string]any{"habit": "跑步"})
	if !strings.Contains(out, `"days_done":1`) || !strings.Contains(out, `"current_streak":1`) {
		t.Errorf("stats: %s", out)
	}
	out, _ = call(t, cs, "health_overview", nil)
	if !strings.Contains(out, `"today_checkins":[{"habit"`) {
		t.Errorf("overview: %s", out)
	}
}

func TestPrompts(t *testing.T) {
	cs, _ := session(t)
	ctx := context.Background()
	list, err := cs.ListPrompts(ctx, nil)
	if err != nil || len(list.Prompts) != len(prompts) {
		t.Fatalf("prompts: %v %v", list, err)
	}
	res, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "symptom_analysis", Arguments: map[string]string{"symptom": "头痛"}})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "「头痛」") || !strings.Contains(text, "search_records") {
		t.Errorf("prompt text: %s", text)
	}
	// every tool a prompt tells Claude to call exists
	tools, _ := cs.ListTools(ctx, nil)
	names := map[string]bool{}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
	}
	for _, p := range prompts {
		for _, w := range strings.FieldsFunc(p.text(map[string]string{"symptom": "x"}), func(r rune) bool {
			return !(r == '_' || r >= 'a' && r <= 'z')
		}) {
			if strings.Contains(w, "_") && !names[w] && w != "type" {
				t.Errorf("prompt %s names unknown tool %q", p.name, w)
			}
		}
	}
}
