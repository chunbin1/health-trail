package mobile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/lee/health-trail/internal/msg"
)

// settings mirrors internal/desktop's settings.json so a data directory
// remains fully compatible between desktop and iOS.
type settings struct {
	path          string
	mu            sync.Mutex
	LAN           bool                  `json:"lan"`
	Lang          string                `json:"lang,omitempty"`
	NoUpdateCheck bool                  `json:"no_update_check,omitempty"`
	NoAutoSave    bool                  `json:"no_autosave,omitempty"`
	HiddenMetrics []string              `json:"hidden_metrics,omitempty"`
	Lists         map[string]listChoice `json:"lists,omitempty"`
}

type listChoice struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
}

func loadSettings(dir string) *settings {
	s := &settings{path: filepath.Join(dir, "settings.json")}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, s)
	}
	return s
}

func (s *settings) lang() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Lang == "en" {
		return "en"
	}
	return "zh"
}

func (s *settings) rawLang() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Lang
}

func (s *settings) msg(zh, en string) string {
	return msg.Pick(s.lang(), zh, en)
}

func (s *settings) errText(err error) string {
	return msg.In(err, s.lang())
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

func (s *settings) lists() map[string]listChoice {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]listChoice{}
	for k, l := range s.Lists {
		out[k] = listChoice{
			Order:  append([]string{}, l.Order...),
			Hidden: append([]string{}, l.Hidden...),
		}
	}
	if _, ok := out["today_metrics"]; !ok && len(s.HiddenMetrics) > 0 {
		out["today_metrics"] = listChoice{
			Order:  []string{},
			Hidden: append([]string{}, s.HiddenMetrics...),
		}
	}
	return out
}

var listNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

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
		if !listNameRe.MatchString(k) {
			return nil, msg.Errorf(
				"lists 的名字 %q 只能用小写字母、数字和下划线，字母开头，最多 40 个字符",
				"the list name %q may only have lowercase letters, digits and underscores, start with a letter, and be at most 40 characters",
				k,
			)
		}
		l.Order, l.Hidden = clean(l.Order), clean(l.Hidden)
		if len(l.Order) > 200 || len(l.Hidden) > 200 {
			return nil, msg.Errorf(
				"lists.%s 的 order 和 hidden 各最多 200 项",
				"lists.%s: order and hidden may have at most 200 each",
				k,
			)
		}
		out[k] = l
	}
	if len(out) > 20 {
		return nil, msg.New("lists 最多 20 个", "at most 20 lists")
	}
	return out, nil
}

func (s *settings) save() error {
	s.mu.Lock()
	b, _ := json.MarshalIndent(s, "", "  ")
	s.mu.Unlock()
	return os.WriteFile(s.path, b, 0o600)
}
