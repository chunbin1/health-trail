package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// listener is the app's socket. It can be moved between 127.0.0.1 and all
// interfaces while the app runs, when phone access is switched on or off.
type listener struct {
	handler http.Handler
	mu      sync.Mutex
	srv     *http.Server
	err     error
}

func (l *listener) listen(addr string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		l.srv.Shutdown(ctx)
		cancel()
		l.srv = nil
	}
	ln, err := net.Listen("tcp", addr)
	l.err = err
	if err != nil {
		return err
	}
	l.srv = &http.Server{
		Addr:              addr,
		Handler:           l.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go l.srv.Serve(ln)
	return nil
}

func (l *listener) addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil {
		return ""
	}
	return l.srv.Addr
}

func (l *listener) errText() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		return ""
	}
	return l.err.Error()
}

func (l *listener) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		l.srv.Close()
		l.srv = nil
	}
}

type settings struct {
	path string
	mu   sync.Mutex // Lang is read by the reminders goroutine
	LAN  bool       `json:"lan"`
	Lang string     `json:"lang,omitempty"` // UI language: "" (Chinese) or "en"
	// looking for updates by itself is on unless switched off
	NoUpdateCheck bool `json:"no_update_check,omitempty"`
	// AI助手 writes a reply as soon as it arrives, unless this is set
	NoAutoSave bool `json:"no_autosave,omitempty"`
	// body metrics left off the 今天 page, from before Lists (read into
	// Lists["today_metrics"] until that is chosen)
	HiddenMetrics []string `json:"hidden_metrics,omitempty"`
	// how the person arranged a list in the UI, by the list's name (as
	// today_metrics, metrics_page); a list not in it is as the page has it
	Lists map[string]listChoice `json:"lists,omitempty"`
}

// listChoice is one list arranged: its items in the order chosen (any not
// in Order come after, as the page has them) and the ones turned off.
type listChoice struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
}

func (s *settings) lang() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Lang
}

// msg picks the text for the UI's language.
func (s *settings) msg(zh, en string) string {
	if s.lang() == "en" {
		return en
	}
	return zh
}

func (s *settings) lists() map[string]listChoice {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]listChoice{}
	for k, l := range s.Lists {
		out[k] = listChoice{append([]string{}, l.Order...), append([]string{}, l.Hidden...)}
	}
	if _, ok := out["today_metrics"]; !ok && len(s.HiddenMetrics) > 0 {
		out["today_metrics"] = listChoice{Order: []string{}, Hidden: append([]string{}, s.HiddenMetrics...)}
	}
	return out
}

func (s *settings) autoUpdate() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.NoUpdateCheck
}

func (s *settings) autoSave() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.NoAutoSave
}

func (s *settings) setAutoSave(on bool) error {
	s.mu.Lock()
	s.NoAutoSave = !on
	s.mu.Unlock()
	return s.save()
}

func loadSettings(dir string) *settings {
	s := &settings{path: filepath.Join(dir, "settings.json")}
	if b, err := os.ReadFile(s.path); err == nil {
		json.Unmarshal(b, s)
	}
	return s
}

var listName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// mergeLists is the lists with the ones in puts replaced, each cleaned
// (names trimmed, each once), or an error saying what is wrong.
func (s *settings) mergeLists(puts map[string]listChoice) (map[string]listChoice, error) {
	out := s.lists()
	clean := func(in []string) []string {
		names := []string{}
		for _, m := range in {
			if m = strings.TrimSpace(m); m != "" && !slices.Contains(names, m) {
				names = append(names, m)
			}
		}
		return names
	}
	for k, l := range puts {
		if !listName.MatchString(k) {
			return nil, fmt.Errorf("lists 的名字 %q 只能用小写字母、数字和下划线，字母开头，最多 40 个字符", k)
		}
		l.Order, l.Hidden = clean(l.Order), clean(l.Hidden)
		if len(l.Order) > 200 || len(l.Hidden) > 200 {
			return nil, fmt.Errorf("lists.%s 的 order 和 hidden 各最多 200 项", k)
		}
		out[k] = l
	}
	if len(out) > 20 {
		return nil, fmt.Errorf("lists 最多 20 个")
	}
	return out, nil
}

func (s *settings) save() error {
	s.mu.Lock()
	b, _ := json.MarshalIndent(s, "", "  ")
	s.mu.Unlock()
	return os.WriteFile(s.path, b, 0o600)
}
