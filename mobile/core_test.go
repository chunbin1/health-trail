package mobile

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestCore(t *testing.T) (*Core, string) {
	t.Helper()
	c := NewCore()
	c.uiOverride = fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>Health Trail</title>")},
	}
	dir := t.TempDir()
	url, err := c.Start(dir)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Stop()
	})
	return c, url
}

func doJSON(t *testing.T, method, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

func TestLifecycle(t *testing.T) {
	c := NewCore()
	c.uiOverride = fstest.MapFS{"index.html": {Data: []byte("ok")}}

	if _, err := c.Start(""); err == nil {
		t.Fatal("expected error for empty dataDir")
	}
	if c.IsRunning() {
		t.Fatal("expected not running")
	}

	dir := t.TempDir()
	url1, err := c.Start(dir)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !strings.HasPrefix(url1, "http://127.0.0.1:") {
		t.Fatalf("unexpected url: %q", url1)
	}
	if !c.IsRunning() || c.BaseURL() != url1 {
		t.Fatalf("expected running at %q, got running=%v url=%q", url1, c.IsRunning(), c.BaseURL())
	}

	// Idempotent start with same dir returns same URL.
	url2, err := c.Start(dir)
	if err != nil || url2 != url1 {
		t.Fatalf("idempotent start: got %q, %v; want %q", url2, err, url1)
	}

	if err := c.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if c.IsRunning() || c.BaseURL() != "" {
		t.Fatalf("expected stopped, got running=%v url=%q", c.IsRunning(), c.BaseURL())
	}
	// Stop again is a no-op.
	if err := c.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestMobileEnvAndInfo(t *testing.T) {
	_, base := newTestCore(t)

	resp, health := doJSON(t, "GET", base+"/api/health", nil)
	if resp.StatusCode != http.StatusOK || health["ok"] != true {
		t.Fatalf("health: status=%d body=%v", resp.StatusCode, health)
	}

	resp, env := doJSON(t, "GET", base+"/api/mobile/env", nil)
	if resp.StatusCode != http.StatusOK || env["mobile"] != true || env["platform"] != "ios" {
		t.Fatalf("mobile/env: status=%d body=%v", resp.StatusCode, env)
	}

	resp, info := doJSON(t, "GET", base+"/api/info", nil)
	if resp.StatusCode != http.StatusOK || info["mobile"] != true || info["platform"] != "ios" {
		t.Fatalf("info: status=%d body=%v", resp.StatusCode, info)
	}
	if info["lan"] != false {
		t.Fatalf("expected lan=false, got %v", info["lan"])
	}
}

func TestSettingsCRUDAndGuard(t *testing.T) {
	_, base := newTestCore(t)

	// Switch language to English.
	resp, info := doJSON(t, "PUT", base+"/api/settings", map[string]any{"lang": "en"})
	if resp.StatusCode != http.StatusOK || info["lang"] != "en" {
		t.Fatalf("put lang=en: status=%d body=%v", resp.StatusCode, info)
	}

	// LAN sharing is rejected on mobile.
	resp, errBody := doJSON(t, "PUT", base+"/api/settings", map[string]any{"lan": true})
	if resp.StatusCode != http.StatusBadRequest || errBody["error"] == "" {
		t.Fatalf("expected 400 for lan=true, got %d %v", resp.StatusCode, errBody)
	}

	// Create and read a condition through SQLite store.
	resp, created := doJSON(t, "POST", base+"/api/r/conditions", map[string]any{
		"name":   "Migraine",
		"status": "进行中",
	})
	if resp.StatusCode != http.StatusCreated || created["name"] != "Migraine" {
		t.Fatalf("create condition: status=%d body=%v", resp.StatusCode, created)
	}

	resp, overview := doJSON(t, "GET", base+"/api/overview", nil)
	if resp.StatusCode != http.StatusOK || overview["today"] == "" {
		t.Fatalf("overview: status=%d body=%v", resp.StatusCode, overview)
	}

	// Cross-origin requests must be refused by server.Guard.
	req, _ := http.NewRequest("GET", base+"/api/info", nil)
	req.Header.Set("Origin", "https://evil.example")
	gResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("guard check: %v", err)
	}
	gResp.Body.Close()
	if gResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin, got %d", gResp.StatusCode)
	}
}

func TestAssistantAndBackupRoutes(t *testing.T) {
	_, base := newTestCore(t)

	// Assistant config & presets
	resp, assist := doJSON(t, "GET", base+"/api/assistant", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/assistant: %d %v", resp.StatusCode, assist)
	}
	if assist["speech"] != false {
		t.Fatalf("expected speech=false, got %v", assist["speech"])
	}
	if providers, ok := assist["providers"].([]any); !ok || len(providers) == 0 {
		t.Fatalf("expected provider presets, got %v", assist["providers"])
	}

	// Update assistant config
	resp, updated := doJSON(t, "PUT", base+"/api/assistant", map[string]any{
		"base_url": "https://open.bigmodel.cn/api/paas/v4",
		"model":    "glm-4.6v-flash",
		"api_key":  "test-key-123",
	})
	if resp.StatusCode != http.StatusOK || updated["configured"] != true || updated["has_key"] != true {
		t.Fatalf("PUT /api/assistant: %d %v", resp.StatusCode, updated)
	}

	// Speech endpoint
	resp, speech := doJSON(t, "GET", base+"/api/assistant/speech", nil)
	if resp.StatusCode != http.StatusOK || speech["available"] != false {
		t.Fatalf("GET /api/assistant/speech: %d %v", resp.StatusCode, speech)
	}

	// Backup config, run, and restore
	backupDir := filepath.Join(t.TempDir(), "backups")
	resp, _ = doJSON(t, "PUT", base+"/api/backup", map[string]any{
		"dir":  backupDir,
		"keep": 3,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/backup: %d", resp.StatusCode)
	}

	resp, runOut := doJSON(t, "POST", base+"/api/backup/run", nil)
	if resp.StatusCode != http.StatusOK || runOut["name"] == "" {
		t.Fatalf("POST /api/backup/run: %d %v", resp.StatusCode, runOut)
	}

	resp, restoreOut := doJSON(t, "POST", base+"/api/backup/restore", map[string]any{"name": runOut["name"]})
	if resp.StatusCode != http.StatusOK || restoreOut["ok"] != true {
		t.Fatalf("POST /api/backup/restore: %d %v", resp.StatusCode, restoreOut)
	}
}

func TestBreakTimerRoutes(t *testing.T) {
	_, base := newTestCore(t)

	// Initial snapshot
	resp, snap := doJSON(t, "GET", base+"/api/break-timer", nil)
	if resp.StatusCode != http.StatusOK || snap["state"] == nil {
		t.Fatalf("GET /api/break-timer: status=%d body=%v", resp.StatusCode, snap)
	}

	// Update config
	cfg, _ := snap["config"].(map[string]any)
	cfg["work_minutes"] = 30
	resp, updated := doJSON(t, "PUT", base+"/api/break-timer/config", cfg)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/break-timer/config: status=%d body=%v", resp.StatusCode, updated)
	}
	if uCfg, ok := updated["config"].(map[string]any); !ok || uCfg["work_minutes"] != float64(30) {
		t.Fatalf("expected work_minutes=30, got %v", updated["config"])
	}

	// Actions: start -> pause -> resume -> break_now -> complete -> water_add -> stop
	for _, action := range []string{"start", "pause", "resume", "break_now", "complete", "water_add", "water_sub", "stop"} {
		resp, out := doJSON(t, "POST", base+"/api/break-timer/action", map[string]string{"action": action})
		if resp.StatusCode != http.StatusOK || out["state"] == nil {
			t.Fatalf("action %q failed: status=%d body=%v", action, resp.StatusCode, out)
		}
	}

	// Invalid action returns 400
	resp, errBody := doJSON(t, "POST", base+"/api/break-timer/action", map[string]string{"action": "bogus"})
	if resp.StatusCode != http.StatusBadRequest || errBody["error"] == "" {
		t.Fatalf("expected 400 for invalid action, got %d %v", resp.StatusCode, errBody)
	}

	// Hide returns ok
	resp, hideOut := doJSON(t, "POST", base+"/api/break-timer/hide", nil)
	if resp.StatusCode != http.StatusOK || hideOut["ok"] != true {
		t.Fatalf("POST /api/break-timer/hide: status=%d body=%v", resp.StatusCode, hideOut)
	}
}

func TestSettingsSaveFailureReturns500(t *testing.T) {
	c, base := newTestCore(t)

	// Force settings.save() to fail by placing a directory at settings.json.
	settingsPath := filepath.Join(c.dataDir, "settings.json")
	_ = os.Remove(settingsPath)
	if err := os.Mkdir(settingsPath, 0o700); err != nil {
		t.Fatalf("mkdir settings.json: %v", err)
	}

	resp, errBody := doJSON(t, "PUT", base+"/api/settings", map[string]any{"lang": "en"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 when settings save fails, got %d body=%v", resp.StatusCode, errBody)
	}
	if msgStr, _ := errBody["error"].(string); msgStr == "" {
		t.Fatalf("expected non-empty error message, got %v", errBody)
	}
	// In-memory language should have been rolled back.
	if got := c.settings.lang(); got != "zh" {
		t.Fatalf("expected lang rolled back to zh, got %q", got)
	}
}
