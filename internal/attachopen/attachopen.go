// Package attachopen opens an attachment from the window in its Mac app.
package attachopen

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// The copies are plain health data, so they don't stay: each open goes to a
// fresh folder, folders older than openedTTL go whenever something is
// opened, an attachment's go when it is deleted, and all of them go when
// the app starts and quits.
const openedTTL = time.Hour

// Dir is where an app's copies go: one folder per app instance (the app
// makes one per data directory), so a test window clearing its own never
// touches the real app's.
func Dir(tmp, instance string) string {
	return filepath.Join(tmp, "health-trail-open", instance)
}

// Clear removes every copy this app made, and the copies an earlier
// version left straight in health-trail-open (one folder per attachment id).
func Clear(base string) {
	os.RemoveAll(base)
	parent := filepath.Dir(base)
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if _, err := strconv.ParseInt(e.Name(), 10, 64); err == nil {
			os.RemoveAll(filepath.Join(parent, e.Name()))
		}
	}
}

// Forget removes the copies of one attachment.
func Forget(base string, id int64) {
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), strconv.FormatInt(id, 10)+"-") {
			os.RemoveAll(filepath.Join(base, e.Name()))
		}
	}
}

func sweepOpened(base string, now time.Time) {
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > openedTTL {
			os.RemoveAll(filepath.Join(base, e.Name()))
		}
	}
}

// Open copies an attachment out under its own name (so the app
// shows that name, and an edit there can't change the record's copy) into
// a fresh folder under base, and opens it, or shows it in the Finder when
// it isn't viewable. run runs `open` with the given arguments.
func Open(st *store.Store, id int64, base string, run func(args ...string) error) (revealed bool, err error) {
	a, err := st.Attachment(id)
	if err != nil {
		return false, err
	}
	sweepOpened(base, time.Now())
	name := filepath.Base(a["filename"].(string))
	if name == "." || name == "/" || strings.HasPrefix(name, ".") {
		name = "attachment" + name
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return false, err
	}
	dir, err := os.MkdirTemp(base, strconv.FormatInt(id, 10)+"-")
	if err != nil {
		return false, err
	}
	dst := filepath.Join(dir, name)
	if err := copyFile(st.AttachmentPath(a), dst); err != nil {
		os.RemoveAll(dir)
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
