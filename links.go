package main

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Clicking the link on an appointment in Calendar or a medicine in Reminders
// (healthtrail://appointments/1, healthtrail://medications/3/202610250800,
// see calsync) opens the window on that record. The link only navigates: it
// is read for a record type and an id, and anything else in it opens the
// window and nothing more.

// linkRoute is the window's route for a link (#/page/entity/id, as in
// frontend/src/state.tsx), or "" when it isn't one of ours.
func linkRoute(link string) string {
	if !strings.HasPrefix(link, syncURL) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(link, syncURL), "/")
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
	return "/" + parts[0] + "/" + parts[0] + "/" + parts[1]
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
		if strings.HasPrefix(s, syncURL) {
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
	if r := linkRoute(link); r != "" {
		js, _ := json.Marshal(r)
		win.ExecJS("location.hash = " + string(js))
	}
}
