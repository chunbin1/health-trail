package desktop

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lee/health-trail/internal/folders"
)

// folderRoutes are the window's: the open dialog pops up on the Mac, so a
// device on the network may not open it, nor change what the app may read.
func (a *app) folderRoutes(mux *http.ServeMux, wapp func() *application.App) {
	mux.HandleFunc("GET /api/folders", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"sandboxed": folders.Sandboxed(), "folders": a.grants.Paths()})
	})
	mux.HandleFunc("POST /api/folders", func(w http.ResponseWriter, r *http.Request) {
		title := a.msg("选择允许读取的文件夹", "Choose a folder to allow")
		if _, err := a.grants.Add(wapp(), title); err != nil {
			// macOS's message can name the folder; it goes to the log only
			log.Printf("添加允许读取的文件夹失败：%v", err)
			writeError(w, a.msg("没能添加这个文件夹，请换一个再试", "Couldn't add that folder; try another one"))
			return
		}
		a.grants.Open() // and this process may read it from now on
		writeJSON(w, map[string]any{"sandboxed": folders.Sandboxed(), "folders": a.grants.Paths()})
	})
	mux.HandleFunc("DELETE /api/folders", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Path == "" {
			writeError(w, "要说明移除哪个文件夹（path）")
			return
		}
		if err := a.grants.Remove(in.Path); err != nil {
			log.Printf("移除允许读取的文件夹失败：%v", err)
			writeError(w, a.msg("没能移除这个文件夹，请再试一次", "Couldn't remove that folder; try again"))
			return
		}
		writeJSON(w, map[string]any{"sandboxed": folders.Sandboxed(), "folders": a.grants.Paths()})
	})
}

// msg is a message for the window, in its language.
func (a *app) msg(zh, en string) string {
	if a.settings.lang() == "en" {
		return en
	}
	return zh
}
