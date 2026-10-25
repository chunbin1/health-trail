package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lee/health-trail/internal/store"
)

const attachInstruction = `
- 用户发来报告单、处方、片子等的截图、PDF、录屏，并且你能拿到它在这台电脑上的文件路径时，录完内容后用 add_attachment 把原文件挂到对应的记录下（报告挂 reports，处方挂 visits 或 medications）。只贴在对话里、没有本机路径的图片存不了，告诉用户可以在 App 里那条记录下自己上传。`

type attachIn struct {
	Path     string `json:"path" jsonschema:"这台电脑上文件的完整路径，如 /Users/me/Downloads/报告.pdf，也可以用 ~/ 开头"`
	Type     string `json:"type" jsonschema:"挂到哪类记录下，如 reports、visits、medications、appointments"`
	ID       int64  `json:"id" jsonschema:"那条记录的 id"`
	Filename string `json:"filename,omitempty" jsonschema:"在档案里显示的文件名，不填就用原文件名"`
}

// attach copies a file on this computer into the data directory as an
// attachment of a record. It reads any file the user can, so New only
// offers it to clients on this computer, never to another device.
func (t *tools) attach(ctx context.Context, _ *mcp.CallToolRequest, in attachIn) (*mcp.CallToolResult, any, error) {
	path := in.Path
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return failed(err)
		}
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		return failed(&store.ValidationError{Msg: fmt.Sprintf("path 要填完整路径（以 / 或 ~/ 开头），收到的是 %q", in.Path)})
	}
	if store.EntityByName(in.Type) == nil {
		return failed(&store.ValidationError{Msg: fmt.Sprintf("没有 %q 这种记录类型，可用的有：%s", in.Type, strings.Join(store.EntityNames(), ", "))})
	}
	owner, err := t.st.Get(in.Type, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return failed(&store.ValidationError{Msg: fmt.Sprintf("%s #%d 不存在，先用 list_records 或 search_records 找到那条记录的 id", in.Type, in.ID)})
	}
	if err != nil {
		return failed(err)
	}
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return failed(&store.ValidationError{Msg: "找不到文件 " + path})
	}
	if err != nil {
		return failed(err)
	}
	if !fi.Mode().IsRegular() {
		return failed(&store.ValidationError{Msg: path + " 不是一个文件（是文件夹或特殊文件）"})
	}
	if fi.Size() > store.MaxAttachment {
		return failed(&store.ValidationError{Msg: fmt.Sprintf("文件有 %.1f MB，附件最大 %d MB", float64(fi.Size())/(1<<20), store.MaxAttachment>>20)})
	}
	f, err := os.Open(path)
	if err != nil {
		return failed(err)
	}
	defer f.Close()

	name := filepath.Base(path)
	if in.Filename != "" {
		name = filepath.Base(in.Filename)
		if filepath.Ext(name) == "" {
			name += filepath.Ext(path)
		}
	}
	// the same rule as an upload in the window: the extension decides, and
	// a file without one is sniffed
	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		ct = http.DetectContentType(head[:n])
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return failed(err)
		}
	}
	a, err := t.st.AddAttachment(in.Type, in.ID, name, ct, f)
	if err != nil {
		return failed(err)
	}
	return result(map[string]any{"attachment": a, "attached_to": map[string]any{"type": in.Type, "id": owner["id"]}})
}
