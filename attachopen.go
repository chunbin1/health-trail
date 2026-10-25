package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lee/health-trail/internal/store"
)

// The window can't follow a link to an attachment: WKWebView opens a new
// window only if the app makes one, and Wails doesn't, so the click does
// nothing. The window asks for the file to be opened in its Mac app instead.

// viewable are the extensions handed to the default app. Anything else is
// only shown in the Finder: `open` on a file without a known extension can
// run it (a script opens in Terminal), and an attachment may be any file.
var viewable = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".heic": true, ".tif": true, ".tiff": true,
	".pdf": true, ".txt": true,
	".mp4": true, ".mov": true, ".m4v": true, ".mp3": true, ".m4a": true, ".wav": true,
}

// openAttachment copies an attachment out under its own name (so the app
// shows that name, and an edit there can't change the record's copy) and
// opens it, or shows it in the Finder when it isn't viewable. run runs
// `open` with the given arguments.
func openAttachment(st *store.Store, id int64, tmp string, run func(args ...string) error) (revealed bool, err error) {
	a, err := st.Attachment(id)
	if err != nil {
		return false, err
	}
	name := filepath.Base(a["filename"].(string))
	if name == "." || name == "/" || strings.HasPrefix(name, ".") {
		name = "attachment" + name
	}
	dir := filepath.Join(tmp, "health-trail-open", strconv.FormatInt(id, 10))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	dst := filepath.Join(dir, name)
	if err := copyFile(st.AttachmentPath(a), dst); err != nil {
		return false, err
	}
	if viewable[strings.ToLower(filepath.Ext(name))] {
		return false, run(dst)
	}
	return true, run("-R", dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (a *app) openAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, "无效的附件 id")
		return
	}
	revealed, err := openAttachment(a.st, id, os.TempDir(), func(args ...string) error {
		return exec.Command("open", args...).Run()
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, "附件不存在")
		return
	}
	if err != nil {
		writeError(w, "打不开附件："+err.Error())
		return
	}
	writeJSON(w, map[string]bool{"revealed": revealed})
}
