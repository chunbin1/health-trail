// Package folders keeps the folders the person let the sandboxed app read.
package folders

import (
	"encoding/json"
	"log"
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

type Grants struct {
	path string
	mu   sync.Mutex
}

func New(dataDir string) *Grants {
	return &Grants{path: filepath.Join(dataDir, "folders.json")}
}

// load and save are called with g.mu held: the window adds and removes
// while the MCP side of the same process may be reading.
func (g *Grants) load() []grant {
	out := []grant{}
	if b, err := os.ReadFile(g.path); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

func (g *Grants) save(list []grant) error {
	if list == nil {
		list = []grant{} // [] in the file, not null
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	return os.WriteFile(g.path, b, 0o600)
}

// open starts this process's access to every granted folder. A bookmark
// macOS calls stale still works now and is made again, so it keeps working.
func (g *Grants) Open() {
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
func (g *Grants) Add(wapp *application.App, title string) (bool, error) {
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

// remove forgets a folder for the next launches. This process keeps reading
// it until it quits: macOS has no way to drop a resolved bookmark's access
// short of stopAccessingSecurityScopedResource, and the next start won't open it.
func (g *Grants) Remove(dir string) error {
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

func (g *Grants) Paths() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := []string{}
	for _, gr := range g.load() {
		out = append(out, gr.Path)
	}
	return out
}

// sandboxed says the app runs in the App Sandbox: there the system points
// the home folder at the app's container.
func Sandboxed() bool {
	home, _ := os.UserHomeDir()
	return strings.Contains(home, "/Library/Containers/")
}
