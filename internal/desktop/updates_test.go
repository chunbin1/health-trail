package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateRoutes(t *testing.T) {
	release := "99.0.0"
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if release == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":"v%[1]s","body":"- 新增","html_url":"https://github.com/chunbin1/health-trail/releases/tag/v%[1]s",
			"assets":[{"name":"HealthTrail-%[1]s.zip","browser_download_url":"https://example.invalid/u.zip","size":3}]}`, release)
	}))
	defer gh.Close()
	dir := t.TempDir()
	u := newUpdates(loadSettings(dir))
	u.url, u.app = gh.URL, "" // a test binary isn't in an .app
	mux := http.NewServeMux()
	u.routes(mux)
	call := func(method, path, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	latest := func(out map[string]any) map[string]any {
		l, _ := out["state"].(map[string]any)["latest"].(map[string]any)
		return l
	}

	_, out := call("GET", "/api/update", "")
	if out["auto"] != true || latest(out) != nil {
		t.Errorf("before a check = %v", out)
	}
	_, out = call("POST", "/api/update/check", "")
	if l := latest(out); l == nil || l["version"] != "99.0.0" || out["installable"] != true || out["why_not"] == "" {
		t.Fatalf("after a check = %v", out)
	}
	// only a copy in an .app can replace itself; the reason is the error
	code, out := call("POST", "/api/update/install", "")
	if code != http.StatusBadRequest || !strings.Contains(fmt.Sprint(out["error"]), "Mac") {
		t.Errorf("install outside an .app = %d %v", code, out)
	}

	// the same version as this build, then none released (a private repo)
	release = Version
	if _, out = call("POST", "/api/update/check", ""); latest(out) != nil {
		t.Errorf("same version offered: %v", out)
	}
	release = ""
	_, out = call("POST", "/api/update/check", "")
	if st := out["state"].(map[string]any); st["latest"] != nil || !strings.Contains(fmt.Sprint(st["error"]), "没有找到") {
		t.Errorf("404 = %v", st)
	}

	// switching the regular check off is kept in settings.json
	if _, out = call("PUT", "/api/update", `{"auto":false}`); out["auto"] != false || u.settings.autoUpdate() {
		t.Errorf("auto off = %v", out)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(raw), `"no_update_check": true`) || loadSettings(dir).autoUpdate() {
		t.Errorf("settings.json = %s", raw)
	}
}

// Checks come every few hours, and on opening the window after a while.
func TestUpdateDue(t *testing.T) {
	now := time.Date(2026, 10, 26, 15, 0, 0, 0, time.Local)
	for _, c := range []struct {
		ago    time.Duration
		onOpen bool
		want   bool
	}{
		{3 * time.Hour, false, false},
		{4 * time.Hour, false, true},
		{29 * time.Minute, true, false},
		{30 * time.Minute, true, true},
	} {
		if got := due(now, now.Add(-c.ago), c.onOpen); got != c.want {
			t.Errorf("last %v ago, on open %v: due = %v, want %v", c.ago, c.onOpen, got, c.want)
		}
	}
	// never checked yet: due either way
	if !due(now, time.Time{}, false) || !due(now, time.Time{}, true) {
		t.Error("no check yet is not due")
	}
}

// The window coming up checks when a check is due, not again soon after,
// and not at all with the checks switched off.
func TestUpdateCheckOnOpen(t *testing.T) {
	var asked atomic.Int32
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		fmt.Fprint(w, `{"tag_name":"v`+Version+`","assets":[]}`)
	}))
	defer gh.Close()
	u := newUpdates(loadSettings(t.TempDir()))
	u.url, u.app = gh.URL, ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go u.loop(ctx)
	wait := func(want int32) {
		t.Helper()
		for i := 0; i < 100 && asked.Load() < want; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond) // and no more than that
		if got := asked.Load(); got != want {
			t.Fatalf("asked GitHub %d times, want %d", got, want)
		}
	}
	u.windowOpened() // none yet: due
	wait(1)
	u.windowOpened() // just checked
	u.windowOpened()
	wait(1)

	u.mu.Lock()
	u.state.CheckedAt = time.Now().Add(-31 * time.Minute).Format(stamp)
	u.mu.Unlock()
	u.settings.mu.Lock()
	u.settings.NoUpdateCheck = true
	u.settings.mu.Unlock()
	u.windowOpened() // due, but switched off
	wait(1)
	u.settings.mu.Lock()
	u.settings.NoUpdateCheck = false
	u.settings.mu.Unlock()
	u.windowOpened()
	wait(2)
}
