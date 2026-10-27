package desktop

import (
	"errors"
	"log"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/lee/health-trail/internal/attachopen"
	"github.com/lee/health-trail/internal/store"
)

// The page shows the message as is, so it names no path; the detail goes to
// the log.
func (a *app) openAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, a.msg("无效的附件 id", "Invalid attachment id"))
		return
	}
	revealed, err := attachopen.Open(a.st, id, a.openDir, func(args ...string) error {
		return exec.Command("open", args...).Run()
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, a.msg("附件不存在", "The attachment doesn't exist"))
		return
	}
	if err != nil {
		log.Printf("打开附件 %d：%v", id, err)
		writeError(w, a.msg("打不开附件", "Couldn't open the attachment"))
		return
	}
	writeJSON(w, map[string]bool{"revealed": revealed})
}

// deleteAttachmentHandler deletes through the API, then the copies the
// window opened of it.
func (a *app) deleteAttachmentHandler(api http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		api.ServeHTTP(w, r)
		if id, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err == nil {
			attachopen.Forget(a.openDir, id)
		}
	}
}
