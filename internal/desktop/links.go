package desktop

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lee/health-trail/internal/calsync"
	"github.com/lee/health-trail/internal/store"
)

// Clicking the link on an appointment in Calendar or a medicine in Reminders
// (healthtrail://appointments/1?uid=…, healthtrail://medications/3/202610250800?uid=…,
// see calsync) opens the window on that record. The link only navigates: it
// is read for a record type, an id and maybe a uid, and anything else in it
// opens the window and nothing more.

// linkRoute is the window's route for a link (#/page/entity/id, as in
// frontend/src/state.tsx), or "" when it isn't one of ours. byUID finds the
// record a uid names in this database; when it does, that's the record (the
// id in the link is the id on the Mac that wrote it), otherwise the id is.
func linkRoute(link string, byUID func(entity, uid string) (int64, bool)) string {
	if !strings.HasPrefix(link, calsync.URLPrefix) {
		return ""
	}
	path, uid := strings.TrimPrefix(link, calsync.URLPrefix), ""
	if i := strings.IndexByte(path, '?'); i >= 0 {
		q := path[i+1:]
		path = path[:i]
		if !strings.HasPrefix(q, "uid=") || !store.ValidUID(q[len("uid="):]) {
			return ""
		}
		uid = q[len("uid="):]
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || !isID(parts[1]) {
		return ""
	}
	switch {
	case parts[0] == "appointments" && len(parts) == 2:
	// a medicine's reminder also names the time it is for
	case parts[0] == "medications" && (len(parts) == 2 || len(parts) == 3 && isStamp(parts[2])):
	default:
		return ""
	}
	id := parts[1]
	if uid != "" && byUID != nil {
		if n, ok := byUID(parts[0], uid); ok {
			id = strconv.FormatInt(n, 10)
		}
	}
	return "/" + parts[0] + "/" + parts[0] + "/" + id
}

// byUID is this database's id for the record with uid.
func (a *app) byUID(entity, uid string) (int64, bool) {
	id, err := a.st.IDByUID(entity, uid)
	return id, err == nil
}

func isID(s string) bool {
	n, err := strconv.ParseUint(s, 10, 32)
	return err == nil && n > 0 && s[0] != '0'
}

// isStamp is a time as calsync writes it, 200601021504.
func isStamp(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// linkIn finds our link among a second launch's arguments, where Wails puts
// the link a copy started by it was opened with.
func linkIn(args []string) string {
	for _, s := range args {
		if strings.HasPrefix(s, calsync.URLPrefix) {
			return s
		}
	}
	return ""
}

// openLink shows the window and, for a link to a record, goes to it. Before
// the page has loaded, Wails holds the script until it has.
func (a *app) openLink(win *application.WebviewWindow, link string) {
	if link != "" {
		log.Printf("打开链接：%q", link)
	}
	win.Restore()
	a.shell.show()
	if r := linkRoute(link, a.byUID); r != "" {
		js, _ := json.Marshal(r)
		win.ExecJS("location.hash = " + string(js))
	}
}
