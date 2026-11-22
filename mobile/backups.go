package mobile

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lee/health-trail/internal/backup"
	"github.com/lee/health-trail/internal/msg"
	"github.com/lee/health-trail/internal/store"
)

type backupManager struct {
	st         *store.Store
	path       string
	defaultDir string
	lang       func() string
	mu         sync.Mutex
}

type backupState struct {
	Config backup.Config `json:"config"`
	Status backupStatus  `json:"status"`
}

type backupStatus struct {
	LastAt      string `json:"last_at,omitempty"`
	LastFile    string `json:"last_file,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	LastErrorEn string `json:"last_error_en,omitempty"`
	ErrorAt     string `json:"error_at,omitempty"`
}

const backupStamp = "2006-01-02 15:04:05"

func newBackupManager(st *store.Store, dataDir string, lang func() string) *backupManager {
	return &backupManager{
		st:         st,
		path:       filepath.Join(dataDir, "backup.json"),
		defaultDir: filepath.Join(filepath.Dir(dataDir), "HealthTrailBackups"),
		lang:       lang,
	}
}

func (b *backupManager) load() backupState {
	var s backupState
	if raw, err := os.ReadFile(b.path); err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	s.Config.Defaults()
	return s
}

func (b *backupManager) save(s backupState) error {
	raw, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(b.path, raw, 0o600)
}

func (b *backupManager) run(suffix string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runLocked(suffix, true)
}

func (b *backupManager) runLocked(suffix string, prune bool) (string, error) {
	s := b.load()
	now := time.Now()
	name, err := backup.Create(b.st, s.Config.Dir, suffix, now)
	if err == nil {
		if prune {
			if perr := backup.Prune(s.Config.Dir, s.Config.Keep); perr != nil {
				log.Printf("mobile backup prune: %v", perr)
			}
		}
		s.Status.LastAt, s.Status.LastFile, s.Status.LastError, s.Status.LastErrorEn, s.Status.ErrorAt = now.Format(backupStamp), name, "", "", ""
	} else {
		s.Status.LastError, s.Status.LastErrorEn, s.Status.ErrorAt = err.Error(), msg.In(err, "en"), now.Format(backupStamp)
		log.Printf("mobile backup failed: %v", err)
	}
	if serr := b.save(s); serr != nil {
		log.Printf("mobile backup save status: %v", serr)
	}
	return name, err
}

func (b *backupManager) restore(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.load()
	list, err := backup.List(s.Config.Dir)
	if err != nil {
		return err
	}
	found := false
	for _, e := range list {
		if e.Name == name {
			found = true
			break
		}
	}
	if !found {
		return msg.New("备份位置里没有这份备份", "That backup isn't in the backup folder")
	}
	if _, err := b.runLocked(backup.BeforeRestore, false); err != nil {
		return msg.Errorf("恢复前先备份当前数据失败，没有恢复：%v", "Couldn't back up the current data first, so nothing was restored: %v", err)
	}
	return backup.Restore(b.st, filepath.Join(s.Config.Dir, name))
}

func (b *backupManager) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		s := b.load()
		list, err := backup.List(s.Config.Dir)
		if err != nil || s.Config.Dir == "" {
			list = []backup.Entry{}
		}
		if b.lang() == "en" && s.Status.LastErrorEn != "" {
			s.Status.LastError = s.Status.LastErrorEn
		}
		writeJSON(w, map[string]any{
			"config":         s.Config,
			"status":         s.Status,
			"backups":        list,
			"suggested_dir":  b.defaultDir,
			"can_choose_dir": false,
		})
	})
	mux.HandleFunc("PUT /api/backup", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		s := b.load()
		if err := json.NewDecoder(r.Body).Decode(&s.Config); err != nil {
			writeError(w, msg.Pick(b.lang(), "无效的请求", "Invalid request"))
			return
		}
		s.Config.Dir = filepath.Clean(s.Config.Dir)
		if s.Config.Dir == "." {
			s.Config.Dir = ""
		}
		if err := s.Config.Check(); err != nil {
			writeError(w, msg.In(err, b.lang()))
			return
		}
		if s.Config.Dir != "" {
			if err := backup.CheckDir(b.st, s.Config.Dir); err != nil {
				writeError(w, msg.In(err, b.lang()))
				return
			}
		}
		if err := b.save(s); err != nil {
			writeError(w, msg.Pick(b.lang(), "保存失败：", "Couldn't save: ")+err.Error())
			return
		}
		writeJSON(w, s.Config)
	})
	mux.HandleFunc("POST /api/backup/run", func(w http.ResponseWriter, r *http.Request) {
		name, err := b.run("")
		if err != nil {
			writeError(w, msg.In(err, b.lang()))
			return
		}
		writeJSON(w, map[string]string{"name": name})
	})
	mux.HandleFunc("POST /api/backup/restore", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
			writeError(w, msg.Pick(b.lang(), "要说明恢复哪一份备份（name）", "Say which backup to restore (name)"))
			return
		}
		if err := b.restore(in.Name); err != nil {
			writeError(w, msg.In(err, b.lang()))
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}
