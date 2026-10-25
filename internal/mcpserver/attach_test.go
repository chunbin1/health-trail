package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lee/health-trail/internal/store"
)

// A report's screenshot and a recording of the scan, saved from paths on
// this computer under the report Claude just entered.
func TestAddAttachment(t *testing.T) {
	cs, st := session(t)
	rep, err := st.Create("reports", store.Record{"report_date": "2026-10-22", "title": "右侧肘关节 MR 平扫", "report_type": "影像"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pdf := filepath.Join(dir, "MR报告.pdf")
	os.WriteFile(pdf, []byte("%PDF-1.4 test"), 0o600)
	png := filepath.Join(dir, "scan") // no extension: the content decides
	os.WriteFile(png, []byte("\x89PNG\r\n\x1a\n0000000000000"), 0o600)

	out, isErr := call(t, cs, "add_attachment", map[string]any{"path": pdf, "type": "reports", "id": rep["id"]})
	if isErr {
		t.Fatal(out)
	}
	out, isErr = call(t, cs, "add_attachment", map[string]any{"path": png, "type": "reports", "id": rep["id"], "filename": "片子截图"})
	if isErr {
		t.Fatal(out)
	}
	as, _ := st.Attachments("reports", rep["id"].(int64))
	if len(as) != 2 {
		t.Fatalf("%d attachments", len(as))
	}
	if as[0]["filename"] != "MR报告.pdf" || as[0]["mime"] != "application/pdf" {
		t.Errorf("pdf saved as %v %v", as[0]["filename"], as[0]["mime"])
	}
	if as[1]["filename"] != "片子截图" || as[1]["mime"] != "image/png" {
		t.Errorf("png saved as %v %v", as[1]["filename"], as[1]["mime"])
	}
	if b, _ := os.ReadFile(st.AttachmentPath(as[0])); string(b) != "%PDF-1.4 test" {
		t.Errorf("stored bytes %q", b)
	}
	if _, err := os.Stat(pdf); err != nil {
		t.Error("the original file should stay where it was")
	}
	// get_record lists them
	if out, _ := call(t, cs, "get_record", map[string]any{"type": "reports", "id": rep["id"]}); !strings.Contains(out, "MR报告.pdf") {
		t.Errorf("get_record: %s", out)
	}

	big := filepath.Join(dir, "big.mp4")
	f, _ := os.Create(big)
	f.Truncate(store.MaxAttachment + 1)
	f.Close()
	for _, c := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"relative path", map[string]any{"path": "MR报告.pdf", "type": "reports", "id": rep["id"]}, "完整路径"},
		{"missing file", map[string]any{"path": filepath.Join(dir, "nope.pdf"), "type": "reports", "id": rep["id"]}, "找不到文件"},
		{"a folder", map[string]any{"path": dir, "type": "reports", "id": rep["id"]}, "不是一个文件"},
		{"too big", map[string]any{"path": big, "type": "reports", "id": rep["id"]}, "附件最大 50 MB"},
		{"no such record", map[string]any{"path": pdf, "type": "reports", "id": 999}, "reports #999 不存在"},
		{"no such type", map[string]any{"path": pdf, "type": "report", "id": rep["id"]}, "可用的有"},
	} {
		out, isErr := call(t, cs, "add_attachment", c.args)
		if !isErr || !strings.Contains(out, c.want) {
			t.Errorf("%s: isErr=%v %s", c.name, isErr, out)
		}
	}
	if as, _ := st.Attachments("reports", rep["id"].(int64)); len(as) != 2 {
		t.Errorf("a refused call saved something: %d attachments", len(as))
	}
}

// add_attachment reads any file on the Mac, so a client on another device
// (the phone, with the token) must not get it, nor be told about it.
func TestAddAttachmentOnlyOnThisComputer(t *testing.T) {
	cs, _ := sessionFor(t, false)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "add_attachment" {
			t.Error("a remote client is offered add_attachment")
		}
	}
	if len(res.Tools) != 14 {
		t.Errorf("%d tools", len(res.Tools))
	}
	if strings.Contains(cs.InitializeResult().Instructions, "add_attachment") {
		t.Error("the instructions mention add_attachment to a remote client")
	}
	local, _ := session(t)
	if !strings.Contains(local.InitializeResult().Instructions, "add_attachment") {
		t.Error("the instructions don't mention add_attachment on this computer")
	}
}
