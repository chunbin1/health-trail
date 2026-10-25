package mcpserver

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
	// what a prompt injection would reach for: keys in a hidden folder,
	// app data in ~/Library, a text file, a file without an extension, and
	// a harmless-looking link to one of them
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	key := filepath.Join(home, ".ssh", "id_ed25519")
	os.WriteFile(key, []byte("-----BEGIN OPENSSH PRIVATE KEY-----"), 0o600)
	os.WriteFile(filepath.Join(home, ".ssh", "photo.jpg"), []byte("\xff\xd8\xff"), 0o600)
	os.MkdirAll(filepath.Join(home, "Library", "Messages"), 0o700)
	os.WriteFile(filepath.Join(home, "Library", "Messages", "x.jpg"), []byte("\xff\xd8\xff"), 0o600)
	notes := filepath.Join(dir, "passwords.txt")
	os.WriteFile(notes, []byte("secret"), 0o600)
	plain := filepath.Join(dir, "token")
	os.WriteFile(plain, []byte("ghp_0123456789abcdef"), 0o600)
	link := filepath.Join(dir, "报告.jpg")
	os.Symlink(filepath.Join(home, ".ssh", "photo.jpg"), link)
	for _, c := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"relative path", map[string]any{"path": "MR报告.pdf", "type": "reports", "id": rep["id"]}, "完整路径"},
		{"missing file", map[string]any{"path": filepath.Join(dir, "nope.pdf"), "type": "reports", "id": rep["id"]}, "找不到这个文件"},
		{"a folder", map[string]any{"path": dir, "type": "reports", "id": rep["id"]}, "不是一个文件"},
		{"too big", map[string]any{"path": big, "type": "reports", "id": rep["id"]}, "附件最大 50 MB"},
		{"no such record", map[string]any{"path": pdf, "type": "reports", "id": 999}, "reports #999 不存在"},
		{"no such type", map[string]any{"path": pdf, "type": "report", "id": rep["id"]}, "可用的有"},
		{"a key", map[string]any{"path": "~/.ssh/id_ed25519", "type": "reports", "id": rep["id"]}, "隐藏"},
		{"a picture in a hidden folder", map[string]any{"path": "~/.ssh/photo.jpg", "type": "reports", "id": rep["id"]}, "隐藏"},
		{"app data", map[string]any{"path": "~/Library/Messages/x.jpg", "type": "reports", "id": rep["id"]}, "资源库"},
		{"a link to a hidden folder", map[string]any{"path": link, "type": "reports", "id": rep["id"]}, "隐藏"},
		{"plain text", map[string]any{"path": notes, "type": "reports", "id": rep["id"]}, "不能存 .txt 文件"},
		{"no extension, not a picture", map[string]any{"path": plain, "type": "reports", "id": rep["id"]}, "没有扩展名"},
	} {
		out, isErr := call(t, cs, "add_attachment", c.args)
		if !isErr || !strings.Contains(out, c.want) {
			t.Errorf("%s: isErr=%v %s", c.name, isErr, out)
		}
		// the result goes into the conversation: no path of this computer
		if strings.Contains(out, dir) || strings.Contains(out, home) {
			t.Errorf("%s: the error names a path: %s", c.name, out)
		}
	}
	// iCloud Drive lives in ~/Library and is fine
	icloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	os.MkdirAll(icloud, 0o700)
	os.WriteFile(filepath.Join(icloud, "体检.pdf"), []byte("%PDF-1.4"), 0o600)
	if out, isErr := call(t, cs, "add_attachment", map[string]any{"path": "~/Library/Mobile Documents/com~apple~CloudDocs/体检.pdf", "type": "reports", "id": rep["id"]}); isErr {
		t.Errorf("iCloud Drive: %s", out)
	}
	// a display name can't change what the file is
	if out, isErr := call(t, cs, "add_attachment", map[string]any{"path": pdf, "type": "reports", "id": rep["id"], "filename": "报告单.jpg"}); isErr || !strings.Contains(out, `"filename":"报告单.pdf"`) || !strings.Contains(out, "application/pdf") {
		t.Errorf("renamed: %s", out)
	}
	if as, _ := st.Attachments("reports", rep["id"].(int64)); len(as) != 4 {
		t.Errorf("a refused call saved something, or an allowed one didn't: %d attachments", len(as))
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

// In the App Store sandbox a file outside Downloads can't be read: the
// error says where to put it instead, and still names no path.
func TestUnreadableInSandbox(t *testing.T) {
	denied := &fs.PathError{Op: "open", Path: "/Users/x/Desktop/体检.pdf", Err: syscall.EPERM}
	sandbox := "/Users/x/Library/Containers/com.chunbin1.healthtrail/Data"
	cases := []struct {
		err        error
		home, want string
	}{
		{denied, sandbox, "「下载」"},
		{denied, "/Users/x", "读不了这个文件"},
		{errors.New("disk on fire"), sandbox, "读不了这个文件"},
	}
	for _, c := range cases {
		res, _, _ := unreadable(c.err, c.home)
		text := res.Content[0].(*mcp.TextContent).Text
		if !res.IsError || !strings.Contains(text, c.want) || strings.Contains(text, "Desktop") || strings.Contains(text, "体检") {
			t.Errorf("unreadable(%v, %q) = %q, want an error with %q and no path", c.err, c.home, text, c.want)
		}
	}
	if inSandbox("/Users/x") || !inSandbox(sandbox) {
		t.Error("inSandbox mistakes the home folder")
	}
}

// In the sandbox $HOME is the app's container, so ~/Library is checked
// against the person's real home as well: a folder they granted may hold it.
func TestAttachKeepsRealLibraryOutInSandbox(t *testing.T) {
	real := t.TempDir()
	if r, err := filepath.EvalSymlinks(real); err == nil {
		real = r
	}
	defer func(f func() string) { realHome = f }(realHome)
	realHome = func() string { return real }
	t.Setenv("HOME", filepath.Join(real, "Library", "Containers", "com.example.app", "Data"))
	os.MkdirAll(os.Getenv("HOME"), 0o700)

	cs, st := session(t)
	rep, _ := st.Create("reports", store.Record{"report_date": "2026-10-01", "title": "体检"})
	put := func(rel string) string {
		p := filepath.Join(real, rel)
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte("%PDF-1.4 test"), 0o600)
		return p
	}
	if out, isErr := call(t, cs, "add_attachment", map[string]any{"path": put("Library/Caches/报告.pdf"), "type": "reports", "id": rep["id"]}); !isErr || !strings.Contains(out, "资源库") {
		t.Errorf("real ~/Library: isErr=%v %s", isErr, out)
	}
	if out, isErr := call(t, cs, "add_attachment", map[string]any{"path": put("Desktop/报告.pdf"), "type": "reports", "id": rep["id"]}); isErr {
		t.Errorf("Desktop: %s", out)
	}
}
