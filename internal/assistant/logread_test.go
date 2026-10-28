package assistant

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The window's log view: newest reply first, the calls and the errors in
// each, and only the end of a big log.
func TestReadLog(t *testing.T) {
	st := openStore(t)
	if v, err := ReadLog(st.Dir); err != nil || len(v.Entries) != 0 {
		t.Fatalf("no log yet: %+v %v", v, err)
	}
	f := &fakeModel{t: t, replies: []string{
		calls("search_records", `{"query":"头痛"}`), answer("第一次的回答"),
		calls("checkin", `{"habit":"喝水"}`), calls("checkin", `{"habit":"喝水"}`),
	}}
	cfg := f.cfg(f.start())
	cfg.Model = "glm-4.6v"
	for _, ask := range []string{"头痛查一下", "打卡喝水"} {
		if _, err := Propose(context.Background(), st, cfg, []Turn{{Role: "user", Text: ask}}, day, nil); err != nil {
			t.Fatal(err)
		}
	}
	v, err := ReadLog(st.Dir)
	if err != nil || len(v.Entries) != 2 || v.Cut {
		t.Fatalf("%+v %v", v, err)
	}
	last, first := v.Entries[0], v.Entries[1]
	if last.Ask != "打卡喝水" || !last.Failed || last.Requests != 2 || last.Model != "glm-4.6v" || last.At == "" ||
		!strings.Contains(last.Text, `调用 checkin {"habit":"喝水"}`) || !strings.Contains(last.Text, "→ 出错：没有叫「喝水」的打卡项") {
		t.Fatalf("last %+v", last)
	}
	if first.Ask != "头痛查一下" || first.Failed || first.Requests != 2 || first.Tokens != 250 || strings.Contains(first.Text, "——") {
		t.Fatalf("first %+v", first)
	}
	if strings.Contains(fmt.Sprint(v), cfg.APIKey) {
		t.Fatal("key in the view")
	}
}

func TestReadLogTail(t *testing.T) {
	dir := t.TempDir()
	entry := func(i int) string {
		return fmt.Sprintf("—— 2026-10-28 10:%02d:00  m  https://x ——\n--- 用户 ---\n第 %d 次喝水%s\n--- 第 1 次请求 · 输入 1 · 输出 1 · 缓存命中 0 ---\ntokens 合计：1 次请求 · 输入 1 · 输出 1 · 合计 2 · 缓存命中 0\n\n",
			i%60, i, strings.Repeat("水", 3000))
	}
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString(entry(i))
	}
	path := filepath.Join(dir, assistantLog)
	os.WriteFile(path, []byte(b.String()), 0o600)
	v, err := ReadLog(dir)
	if err != nil || !v.Cut || v.Size != int64(b.Len()) {
		t.Fatalf("%v %v %d", err, v.Cut, v.Size)
	}
	// only whole replies, the newest first, and no broken characters
	if len(v.Entries) == 0 || len(v.Entries) >= 100 || !strings.HasPrefix(v.Entries[0].Ask, "第 99 次喝水") {
		t.Fatalf("%d entries, first %q", len(v.Entries), v.Entries[0].Ask)
	}
	for _, e := range v.Entries {
		if e.At == "" || e.Requests != 1 || !strings.HasPrefix(e.Text, "--- 用户 ---") {
			t.Fatalf("partial entry %+v", e.Ask)
		}
	}

	// just started again: the end of the old log comes before the new one
	os.Rename(path, path+".1")
	os.WriteFile(path, []byte(entry(100)), 0o600)
	v, err = ReadLog(dir)
	if err != nil || !v.Cut || len(v.Entries) < 2 || !strings.HasPrefix(v.Entries[0].Ask, "第 100 次") || !strings.HasPrefix(v.Entries[1].Ask, "第 99 次") {
		t.Fatalf("after rotation: %v %d", err, len(v.Entries))
	}
}
