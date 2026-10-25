package desktop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateRoutes(t *testing.T) {
	release := "99.0.0"
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if release == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":"v%[1]s","body":"- 新增","html_url":"https://github.com/chunbin1/health-vault/releases/tag/v%[1]s",
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

	// switching the daily check off is kept in settings.json
	if _, out = call("PUT", "/api/update", `{"auto":false}`); out["auto"] != false || u.settings.autoUpdate() {
		t.Errorf("auto off = %v", out)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(raw), `"no_update_check": true`) || loadSettings(dir).autoUpdate() {
		t.Errorf("settings.json = %s", raw)
	}
}
