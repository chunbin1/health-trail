package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lee/health-vault/internal/backup"
	"github.com/lee/health-vault/internal/store"
)

// backups runs the backups the person set up: when they're due, when the
// data has settled after a change, and on quit. Its settings and the last
// result live in backup.json in the data folder, read afresh each time, so
// two processes on the same data (the app and a dev build) see each other's
// backups instead of both making one.
type backups struct {
	st   *store.Store
	path string
	mu   sync.Mutex // one backup or restore at a time
}

type backupState struct {
	Config backup.Config `json:"config"`
	Status backupStatus  `json:"status"`
}

type backupStatus struct {
	LastAt    string `json:"last_at,omitempty"` // YYYY-MM-DD HH:MM:SS
	LastFile  string `json:"last_file,omitempty"`
	LastError string `json:"last_error,omitempty"`
	ErrorAt   string `json:"error_at,omitempty"`
}

const stamp = "2006-01-02 15:04:05"

func newBackups(st *store.Store, dataDir string) *backups {
	return &backups{st: st, path: filepath.Join(dataDir, "backup.json")}
}

func (b *backups) load() backupState {
	var s backupState
	if raw, err := os.ReadFile(b.path); err == nil {
		json.Unmarshal(raw, &s)
	}
	s.Config.Defaults()
	return s
}

func (b *backups) save(s backupState) error {
	raw, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(b.path, raw, 0o600)
}

// run makes a backup now and records how it went.
func (b *backups) run(suffix string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runLocked(suffix)
}

func (b *backups) runLocked(suffix string) (string, error) {
	s := b.load()
	now := time.Now()
	name, err := backup.Create(b.st, s.Config.Dir, suffix, now)
	if err == nil {
		if perr := backup.Prune(s.Config.Dir, s.Config.Keep); perr != nil {
			log.Printf("清理旧备份失败：%v", perr)
		}
		s.Status.LastAt, s.Status.LastFile, s.Status.LastError, s.Status.ErrorAt = now.Format(stamp), name, "", ""
	} else {
		s.Status.LastError, s.Status.ErrorAt = err.Error(), now.Format(stamp)
		log.Printf("备份失败：%v", err)
	}
	if serr := b.save(s); serr != nil {
		log.Printf("保存备份状态失败：%v", serr)
	}
	return name, err
}

// restore backs up the current data, then restores the named backup.
func (b *backups) restore(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.load()
	list, err := backup.List(s.Config.Dir)
	if err != nil {
		return err
	}
	found := false
	for _, e := range list {
		found = found || e.Name == name
	}
	if !found {
		return errors.New("备份位置里没有这份备份")
	}
	if _, err := b.runLocked(backup.BeforeRestore); err != nil {
		return errors.New("恢复前先备份当前数据失败，没有恢复：" + err.Error())
	}
	return backup.Restore(b.st, filepath.Join(s.Config.Dir, name))
}

// loop checks every 30 seconds for a backup that's due, and watches the data
// for the on-change trigger: PRAGMA data_version on a connection of its own
// moves whenever any other connection commits (this app, Claude over MCP, a
// dev build), and a checkpoint doesn't move it.
func (b *backups) loop(ctx context.Context) {
	conn, err := b.st.DB.Conn(ctx)
	if err != nil {
		log.Printf("备份检查没有启动：%v", err)
		return
	}
	defer conn.Close()
	version := func() int64 {
		var v int64
		conn.QueryRowContext(ctx, "PRAGMA data_version").Scan(&v)
		return v
	}
	seen, changedAt := version(), time.Time{}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		s := b.load()
		now := time.Now()
		if v := version(); v != seen {
			seen, changedAt = v, now
		}
		if s.Config.Dir != "" {
			// a failed attempt counts as one: an error isn't retried every 30
			// seconds, only at the next scheduled time or change
			tried := parseStamp(s.Status.LastAt)
			if e := parseStamp(s.Status.ErrorAt); e.After(tried) {
				tried = e
			}
			quiet := s.Config.OnChange && !changedAt.IsZero() && now.Sub(changedAt) >= backup.Quiet
			if quiet || backup.Due(s.Config, tried, now) {
				b.run("")
				changedAt = time.Time{}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func parseStamp(s string) time.Time {
	t, _ := time.ParseInLocation(stamp, s, time.Local)
	return t
}

// quit makes the on-quit backup, if that's switched on.
func (b *backups) quit() {
	if s := b.load(); s.Config.OnQuit && s.Config.Dir != "" {
		b.run("")
	}
}

// routes are the backup API: settings and list, back up now, restore.
func (b *backups) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		s := b.load()
		list, err := backup.List(s.Config.Dir)
		if err != nil || s.Config.Dir == "" {
			list = []backup.Entry{}
		}
		writeJSON(w, map[string]any{
			"config":         s.Config,
			"status":         s.Status,
			"backups":        list,
			"suggested_dir":  suggestedBackupDir(),
			"can_choose_dir": r.Context().Value(inWindow{}) != nil,
		})
	})
	mux.HandleFunc("PUT /api/backup", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		s := b.load()
		if err := json.NewDecoder(r.Body).Decode(&s.Config); err != nil {
			writeError(w, "无效的请求")
			return
		}
		s.Config.Dir = filepath.Clean(s.Config.Dir)
		if s.Config.Dir == "." {
			s.Config.Dir = ""
		}
		if err := s.Config.Check(); err != nil {
			writeError(w, err.Error())
			return
		}
		if s.Config.Dir != "" {
			if err := backup.CheckDir(b.st, s.Config.Dir); err != nil {
				writeError(w, err.Error())
				return
			}
		}
		if err := b.save(s); err != nil {
			writeError(w, "保存失败："+err.Error())
			return
		}
		writeJSON(w, s.Config)
	})
	mux.HandleFunc("POST /api/backup/run", func(w http.ResponseWriter, r *http.Request) {
		name, err := b.run("")
		if err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, map[string]string{"name": name})
	})
	mux.HandleFunc("POST /api/backup/restore", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
			writeError(w, "要说明恢复哪一份备份（name）")
			return
		}
		if err := b.restore(in.Name); err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}

// suggestedBackupDir is a folder in iCloud Drive, when it's set up: off this
// disk, and synced to the person's other devices.
func suggestedBackupDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	icloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	if info, err := os.Stat(icloud); err == nil && info.IsDir() {
		return filepath.Join(icloud, "HealthVaultBackups")
	}
	return ""
}

// inWindow marks a request that came from the app's own window, not over the
// network: only those may open a dialog on the Mac.
type inWindow struct{}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
