package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Folders the person let the app read. In the App Store build the sandbox
// lets it read only its own container and Downloads, so Claude couldn't
// attach a report from the Desktop. A folder picked in the window's open
// dialog is granted to the app; a security-scoped bookmark keeps that
// across launches, and the MCP process (the same app, launched by Claude)
// opens the same bookmarks at its start, so add_attachment reads there too.
// Outside the sandbox nothing is restricted and none of this is shown.

type grant struct {
	Path     string `json:"path"`
	Bookmark []byte `json:"bookmark"`
}

type grants struct {
	path string
	mu   sync.Mutex
}

func newGrants(dataDir string) *grants {
	return &grants{path: filepath.Join(dataDir, "folders.json")}
}

func (g *grants) load() []grant {
	var out []grant
	if b, err := os.ReadFile(g.path); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

func (g *grants) save(list []grant) error {
	b, _ := json.MarshalIndent(list, "", "  ")
	return os.WriteFile(g.path, b, 0o600)
}

// open starts this process's access to every granted folder. A bookmark
// macOS calls stale still works now and is made again, so it keeps working.
func (g *grants) open() {
	g.mu.Lock()
	defer g.mu.Unlock()
	list, changed := g.load(), false
	for i, gr := range list {
		path, stale, err := openBookmark(gr.Bookmark)
		if err != nil {
			log.Printf("允许读取的文件夹 %s 打不开了：%v", gr.Path, err)
			continue
		}
		if stale {
			if b, err := makeBookmark(path); err == nil {
				list[i].Bookmark, list[i].Path, changed = b, path, true
			}
		}
	}
	if changed {
		if err := g.save(list); err != nil {
			log.Printf("更新文件夹授权失败：%v", err)
		}
	}
}

// add asks for a folder in the open dialog, which grants it, and keeps it.
// It returns false when the dialog was cancelled.
func (g *grants) add(wapp *application.App, title string) (bool, error) {
	dir, err := wapp.Dialog.OpenFile().SetTitle(title).
		CanChooseDirectories(true).CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil || dir == "" {
		return false, err
	}
	b, err := makeBookmark(dir)
	if err != nil {
		return false, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	list := g.load()
	kept := list[:0]
	for _, gr := range list {
		if gr.Path != dir {
			kept = append(kept, gr)
		}
	}
	return true, g.save(append(kept, grant{Path: dir, Bookmark: b}))
}

func (g *grants) remove(dir string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	list := g.load()
	kept := list[:0]
	for _, gr := range list {
		if gr.Path != dir {
			kept = append(kept, gr)
		}
	}
	return g.save(kept)
}

func (g *grants) paths() []string {
	out := []string{}
	for _, gr := range g.load() {
		out = append(out, gr.Path)
	}
	return out
}

// sandboxed says the app runs in the App Sandbox: there the system points
// the home folder at the app's container.
func sandboxed() bool {
	home, _ := os.UserHomeDir()
	return strings.Contains(home, "/Library/Containers/")
}

// folderRoutes are the window's: the open dialog pops up on the Mac, so a
// device on the network may not open it, nor change what the app may read.
func (a *app) folderRoutes(mux *http.ServeMux, wapp func() *application.App) {
	mux.HandleFunc("GET /api/folders", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"sandboxed": sandboxed(), "folders": a.grants.paths()})
	})
	mux.HandleFunc("POST /api/folders", func(w http.ResponseWriter, r *http.Request) {
		title := "选择允许读取的文件夹"
		if a.settings.lang() == "en" {
			title = "Choose a folder to allow"
		}
		if _, err := a.grants.add(wapp(), title); err != nil {
			writeError(w, err.Error())
			return
		}
		a.grants.open() // and this process may read it from now on
		writeJSON(w, map[string]any{"sandboxed": sandboxed(), "folders": a.grants.paths()})
	})
	mux.HandleFunc("DELETE /api/folders", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Path == "" {
			writeError(w, "要说明移除哪个文件夹（path）")
			return
		}
		if err := a.grants.remove(in.Path); err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, map[string]any{"sandboxed": sandboxed(), "folders": a.grants.paths()})
	})
}
