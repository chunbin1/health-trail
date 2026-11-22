package desktop

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lee/health-trail/internal/breaktimer"
)

func TestBreakTimerHTTP(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, "0")
	defer a.st.Close()
	h := a.handler()

	// Initial GET snapshot
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/break-timer", nil))
	if w.Code != 200 {
		t.Fatalf("GET /api/break-timer status=%d body=%s", w.Code, w.Body.String())
	}
	var snap breaktimer.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.State != breaktimer.StateIdle || snap.Config.WorkMinutes != 45 {
		t.Fatalf("unexpected initial snapshot: %+v", snap)
	}

	// Start action
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/break-timer/action", strings.NewReader(`{"action":"start"}`)))
	if w.Code != 200 {
		t.Fatalf("POST start status=%d body=%s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &snap)
	if snap.State != breaktimer.StateWorking {
		t.Fatalf("after start state=%s", snap.State)
	}

	// Water add
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/break-timer/action", strings.NewReader(`{"action":"water_add"}`)))
	json.Unmarshal(w.Body.Bytes(), &snap)
	if snap.Today.Water != 1 {
		t.Fatalf("after water_add water=%d", snap.Today.Water)
	}

	// Unknown action -> 400
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/break-timer/action", strings.NewReader(`{"action":"bogus"}`)))
	if w.Code != 400 {
		t.Fatalf("unknown action status=%d, want 400", w.Code)
	}

	// Invalid config -> 400 in Chinese, then switch lang=en -> 400 in English
	badCfg := `{"work_minutes":2,"break_minutes":3,"snooze_minutes":5,"water_goal":8,"auto_schedule":false,"schedule_start":"09:00","schedule_end":"18:30","weekdays_only":true,"popup_enabled":true,"sound_enabled":true,"menu_bar_countdown":true}`
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("PUT", "/api/break-timer/config", strings.NewReader(badCfg)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "工作时长") {
		t.Fatalf("expected zh validation error, got %d %s", w.Code, w.Body.String())
	}

	a.settings.mu.Lock()
	a.settings.Lang = "en"
	a.settings.mu.Unlock()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("PUT", "/api/break-timer/config", strings.NewReader(badCfg)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "Work duration") {
		t.Fatalf("expected en validation error, got %d %s", w.Code, w.Body.String())
	}

	// Hide route works cleanly without a window
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/break-timer/hide", nil))
	if w.Code != 200 {
		t.Fatalf("POST hide status=%d", w.Code)
	}
}

func TestBreakTimerHTTPSaveError(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, "0")
	defer a.st.Close()
	h := a.handler()

	// Replace breaktimer.json path with a directory to force save failure.
	if err := os.Mkdir(filepath.Join(dir, "breaktimer.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/break-timer/action", strings.NewReader(`{"action":"water_add"}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "保存休息提醒数据失败") {
		t.Fatalf("expected zh save error, got %d %s", w.Code, w.Body.String())
	}

	a.settings.mu.Lock()
	a.settings.Lang = "en"
	a.settings.mu.Unlock()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/break-timer/action", strings.NewReader(`{"action":"water_add"}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "Failed to save break timer data") {
		t.Fatalf("expected en save error, got %d %s", w.Code, w.Body.String())
	}
}
