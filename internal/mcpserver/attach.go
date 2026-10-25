package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

// attachExts are the kinds of file a health record keeps, and the only ones
// add_attachment reads. Plain text and anything without an extension (keys,
// .env files, notes) are left out: a prompt injection that gets Claude to
// attach one would copy a secret into the records and into every backup.
// A file without an extension still goes in when its content is a picture,
// a PDF or a recording.
var attachExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".heic": true, ".heif": true, ".tif": true, ".tiff": true, ".bmp": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".dcm": true,
	".mp4": true, ".mov": true, ".m4v": true, ".mp3": true, ".m4a": true, ".wav": true,
}

func sniffedOK(ct string) bool {
	return ct == "application/pdf" || strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/")
}

// attachable says why a file (its real path, links followed) may not be
// read, or "" when it may. Hidden files and folders hold keys and settings,
// and ~/Library holds every app's data, so neither is read, except iCloud
// Drive and the other cloud folders kept in ~/Library.
func attachable(real, home string) string {
	for _, part := range strings.Split(real, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") {
			return "不能存隐藏文件，或隐藏文件夹（名字以 . 开头）里的文件"
		}
	}
	lib := filepath.Join(home, "Library")
	if home != "" && (real == lib || strings.HasPrefix(real, lib+string(filepath.Separator))) {
		for _, ok := range []string{"Mobile Documents", "CloudStorage"} {
			if strings.HasPrefix(real, filepath.Join(lib, ok)+string(filepath.Separator)) {
				return ""
			}
		}
		return "不能存「资源库」（~/Library）里的文件，iCloud 云盘里的可以"
	}
	return ""
}

// unreadable reports a failure to read a file without its path: the result
// goes into the conversation, which leaves this computer. The detail goes
// to the log (stderr) only.
func unreadable(err error) (*mcp.CallToolResult, any, error) {
	log.Printf("add_attachment: %v", err)
	return failed(&store.ValidationError{Msg: "读不了这个文件（可能没有权限）"})
}

// attach copies a file on this computer into the data directory as an
// attachment of a record. It reads files on this computer, so New only
// offers it to clients on this computer, never to another device, and only
// the kinds of file in attachExts, outside hidden folders and ~/Library.
// That is still any such file the user can read: a residual risk of letting
// a local Claude attach files at all. No error names a path.
func (t *tools) attach(ctx context.Context, _ *mcp.CallToolRequest, in attachIn) (*mcp.CallToolResult, any, error) {
	home, _ := os.UserHomeDir()
	path := in.Path
	if rest, ok := strings.CutPrefix(path, "~/"); ok && home != "" {
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		return failed(&store.ValidationError{Msg: "path 要填完整路径（以 / 或 ~/ 开头）"})
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
	// judge the file a link points to, not the link
	real, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		return failed(&store.ValidationError{Msg: "找不到这个文件，请确认 path 是对的"})
	}
	if err != nil {
		return unreadable(err)
	}
	if home != "" {
		if h, err := filepath.EvalSymlinks(home); err == nil {
			home = h
		}
	}
	if why := attachable(real, home); why != "" {
		return failed(&store.ValidationError{Msg: why})
	}
	fi, err := os.Stat(real)
	if err != nil {
		return unreadable(err)
	}
	if !fi.Mode().IsRegular() {
		return failed(&store.ValidationError{Msg: "path 指向的不是一个文件（是文件夹或特殊文件）"})
	}
	ext := strings.ToLower(filepath.Ext(real))
	if ext != "" && !attachExts[ext] {
		return failed(&store.ValidationError{Msg: fmt.Sprintf("不能存 %s 文件。可以存的：图片、PDF、Word/Excel、DICOM、视频和录音（%s）", ext, extList())})
	}
	if fi.Size() > store.MaxAttachment {
		return failed(&store.ValidationError{Msg: fmt.Sprintf("文件有 %.1f MB，附件最大 %d MB", float64(fi.Size())/(1<<20), store.MaxAttachment>>20)})
	}
	f, err := os.Open(real)
	if err != nil {
		return unreadable(err)
	}
	defer f.Close()

	// the type comes from the file's own extension, as an upload in the
	// window does; a file without one is sniffed
	ct := mime.TypeByExtension(ext)
	if ext == "" {
		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		ct = http.DetectContentType(head[:n])
		if !sniffedOK(ct) {
			return failed(&store.ValidationError{Msg: "这个文件没有扩展名，内容也不是图片、PDF 或视频，不能存"})
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return unreadable(err)
		}
	}
	// a display name keeps the file's real extension, so the type shown
	// and the way it opens match what the file is
	name := filepath.Base(path)
	if in.Filename != "" {
		name = filepath.Base(in.Filename)
		name = strings.TrimSuffix(name, filepath.Ext(name)) + filepath.Ext(real)
	}
	a, err := t.st.AddAttachment(in.Type, in.ID, name, ct, f)
	var verr *store.ValidationError
	if err != nil && !errors.As(err, &verr) {
		log.Printf("add_attachment: %v", err)
		return failed(&store.ValidationError{Msg: "保存附件失败"})
	}
	if err != nil {
		return failed(err)
	}
	return result(map[string]any{"attachment": a, "attached_to": map[string]any{"type": in.Type, "id": owner["id"]}})
}

func extList() string {
	exts := make([]string, 0, len(attachExts))
	for e := range attachExts {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	return strings.Join(exts, " ")
}
