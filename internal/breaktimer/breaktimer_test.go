package breaktimer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lee/health-trail/internal/msg"
)

type fakeClock struct {
	t time.Time
}

func (c *fakeClock) Now() time.Time { return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func mustSnap(t *testing.T) func(Snapshot, error) Snapshot {
	t.Helper()
	return func(snap Snapshot, err error) Snapshot {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return snap
	}
}

func TestCycleAndBreakDoneWaitsForUser(t *testing.T) {
	must := mustSnap(t)
	clk := &fakeClock{t: time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)} // Monday
	e := New(t.TempDir(), clk.Now)

	var events []Event
	e.SetOnChange(func(ev Event) {
		events = append(events, ev)
	})

	snap := must(e.Start())
	if snap.State != StateWorking || snap.RemainingSec != 45*60 || snap.TotalSec != 45*60 {
		t.Fatalf("after Start: %+v", snap)
	}

	// Advance second by second through the work period.
	for i := 0; i < 45*60-1; i++ {
		clk.Advance(time.Second)
		ev := e.Tick()
		if ev.Snapshot.State != StateWorking || ev.OpenBreak {
			t.Fatalf("premature transition at step %d: %+v", i, ev)
		}
	}

	// Last second triggers break.
	clk.Advance(time.Second)
	ev := e.Tick()
	if ev.Snapshot.State != StateBreak || !ev.OpenBreak || !ev.PlaySound || ev.Snapshot.BreakSeq != 1 {
		t.Fatalf("expected break transition: %+v", ev)
	}
	if ev.Snapshot.RemainingSec != 3*60 {
		t.Fatalf("break remaining=%d, want 180", ev.Snapshot.RemainingSec)
	}

	// Advance through break duration.
	for i := 0; i < 3*60; i++ {
		clk.Advance(time.Second)
		ev = e.Tick()
	}
	if ev.Snapshot.State != StateBreakDone {
		t.Fatalf("after break countdown: state=%s, want break_done", ev.Snapshot.State)
	}
	if ev.HideBreak {
		t.Fatalf("break_done must NOT auto-hide break window")
	}
	if ev.Snapshot.Today.Breaks != 0 {
		t.Fatalf("break_done must NOT auto-increment Breaks, got %d", ev.Snapshot.Today.Breaks)
	}

	// Stay in break_done even after ticking several more minutes.
	for i := 0; i < 120; i++ {
		clk.Advance(time.Second)
		ev = e.Tick()
		if ev.Snapshot.State != StateBreakDone || ev.HideBreak || ev.Snapshot.Today.Breaks != 0 {
			t.Fatalf("break_done should remain waiting: %+v", ev)
		}
	}

	// User clicks "开始下一轮".
	snap = must(e.CompleteBreak(true))
	if snap.State != StateWorking || snap.Today.Breaks != 1 || snap.RemainingSec != 45*60 {
		t.Fatalf("after CompleteBreak(true): %+v", snap)
	}
	last := events[len(events)-1]
	if !last.HideBreak {
		t.Fatalf("CompleteBreak should emit HideBreak=true")
	}
}

func TestCompleteIdleSnoozeSkipPauseWater(t *testing.T) {
	must := mustSnap(t)
	clk := &fakeClock{t: time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)}
	dir := t.TempDir()
	e := New(dir, clk.Now)

	// Water increments and clamps at 0.
	snap := must(e.AddWater(-1))
	if snap.Today.Water != 0 {
		t.Fatalf("water below 0: %d", snap.Today.Water)
	}
	snap = must(e.AddWater(1))
	snap = must(e.AddWater(1))
	snap = must(e.AddWater(-1))
	if snap.Today.Water != 1 {
		t.Fatalf("water=%d, want 1", snap.Today.Water)
	}

	// Pause and resume preserves remaining time.
	must(e.Start())
	clk.Advance(10 * time.Second)
	snap = e.Pause()
	if snap.State != StatePaused || snap.RemainingSec != 45*60-10 {
		t.Fatalf("paused snap: %+v", snap)
	}
	clk.Advance(5 * time.Minute)
	if got := e.Snapshot().RemainingSec; got != 45*60-10 {
		t.Fatalf("paused drifted to %d", got)
	}
	snap = must(e.Resume())
	if snap.State != StateWorking || snap.RemainingSec != 45*60-10 {
		t.Fatalf("resumed snap: %+v", snap)
	}

	// Snooze during working adds snooze minutes.
	snap = must(e.Snooze())
	if snap.State != StateWorking || snap.RemainingSec != 50*60-10 {
		t.Fatalf("snooze during working: %+v", snap)
	}

	// Break now -> Snooze during break hides break and starts snooze countdown.
	snap = must(e.StartBreakNow())
	if snap.State != StateBreak || snap.BreakSeq != 1 {
		t.Fatalf("StartBreakNow: %+v", snap)
	}
	var lastEv Event
	e.SetOnChange(func(ev Event) { lastEv = ev })
	snap = must(e.Snooze())
	if snap.State != StateWorking || snap.RemainingSec != 5*60 || !lastEv.HideBreak {
		t.Fatalf("snooze during break: snap=%+v ev=%+v", snap, lastEv)
	}

	// Break now -> Skip does NOT count break and starts fresh work round.
	must(e.StartBreakNow())
	snap = must(e.SkipBreak())
	if snap.State != StateWorking || snap.Today.Breaks != 0 || snap.RemainingSec != 45*60 || !lastEv.HideBreak {
		t.Fatalf("SkipBreak: snap=%+v ev=%+v", snap, lastEv)
	}

	// Break now -> CompleteBreak(false) counts break and goes idle.
	must(e.StartBreakNow())
	snap = must(e.CompleteBreak(false))
	if snap.State != StateIdle || snap.Today.Breaks != 1 || !lastEv.HideBreak {
		t.Fatalf("CompleteBreak(false): snap=%+v ev=%+v", snap, lastEv)
	}

	// Persistence round-trip.
	cfg := DefaultConfig()
	cfg.WorkMinutes = 50
	cfg.BreakMinutes = 4
	if err := e.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	e2 := New(dir, clk.Now)
	snap2 := e2.Snapshot()
	if snap2.Config.WorkMinutes != 50 || snap2.Config.BreakMinutes != 4 || snap2.Today.Breaks != 1 || snap2.Today.Water != 1 {
		t.Fatalf("reloaded snapshot mismatch: %+v", snap2)
	}
	if len(snap2.History) != 7 || snap2.History[6].Date != "2026-11-16" || snap2.History[6].Breaks != 1 {
		t.Fatalf("history mismatch: %+v", snap2.History)
	}
}

func TestSnoozeDuringBreakDoneIsNoOp(t *testing.T) {
	must := mustSnap(t)
	clk := &fakeClock{t: time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)}
	e := New(t.TempDir(), clk.Now)

	must(e.StartBreakNow())
	clk.Advance(3 * time.Minute)
	ev := e.Tick()
	if ev.Snapshot.State != StateBreakDone {
		t.Fatalf("expected break_done, got %s", ev.Snapshot.State)
	}

	fired := false
	e.SetOnChange(func(ev Event) { fired = true })
	snap := must(e.Snooze())
	if snap.State != StateBreakDone || fired {
		t.Fatalf("Snooze during break_done must be a no-op: snap=%+v fired=%v", snap, fired)
	}
}

func TestManualStopPersistsAcrossRestart(t *testing.T) {
	must := mustSnap(t)
	clk := &fakeClock{t: time.Date(2026, 11, 23, 10, 0, 0, 0, time.Local)} // Monday inside 09:00-18:30
	dir := t.TempDir()
	e := New(dir, clk.Now)
	cfg := DefaultConfig()
	cfg.AutoSchedule = true
	if err := e.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if got := e.Snapshot().State; got != StateWorking {
		t.Fatalf("expected working inside schedule, got %s", got)
	}

	// Stop manually, then restart app same day inside schedule -> must remain idle.
	if snap := must(e.Stop()); snap.State != StateIdle {
		t.Fatalf("after Stop: %s", snap.State)
	}
	e2 := New(dir, clk.Now)
	if got := e2.Snapshot().State; got != StateIdle {
		t.Fatalf("after restart same day post-Stop: got %s, want idle", got)
	}
	if got := e2.Tick().Snapshot.State; got != StateIdle {
		t.Fatalf("after Tick post-restart: got %s, want idle", got)
	}

	// Starting break and choosing "暂不开始" (CompleteBreak(false)) also persists across restart.
	must(e2.StartBreakNow())
	if snap := must(e2.CompleteBreak(false)); snap.State != StateIdle {
		t.Fatalf("after CompleteBreak(false): %s", snap.State)
	}
	e3 := New(dir, clk.Now)
	if got := e3.Snapshot().State; got != StateIdle {
		t.Fatalf("after restart same day post-CompleteBreak(false): got %s, want idle", got)
	}

	// Next day inside schedule -> auto-starts working again.
	clk.t = time.Date(2026, 11, 24, 9, 5, 0, 0, time.Local)
	e4 := New(dir, clk.Now)
	if got := e4.Snapshot().State; got != StateWorking {
		t.Fatalf("next day inside schedule should auto-start working, got %s", got)
	}

	// Manual Stop then manual Start clears manualStoppedDate on disk too.
	must(e4.Stop())
	must(e4.Start())
	e5 := New(dir, clk.Now)
	if got := e5.Snapshot().State; got != StateWorking {
		t.Fatalf("restart after manual Start should enter working, got %s", got)
	}
}

func TestSaveFailureReturnsErrorAndRollsBack(t *testing.T) {
	must := mustSnap(t)
	clk := &fakeClock{t: time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)}
	dir := t.TempDir()
	// Create a directory at breaktimer.json so os.Rename(tmp, e.path) fails cleanly on all platforms.
	badTarget := filepath.Join(dir, "breaktimer.json")
	if err := os.Mkdir(badTarget, 0o700); err != nil {
		t.Fatal(err)
	}

	e := New(dir, clk.Now)

	// AddWater fails and rolls back in-memory count.
	snap, err := e.AddWater(1)
	if err == nil {
		t.Fatal("expected AddWater error when breaktimer.json is a directory")
	}
	if !strings.Contains(err.Error(), "保存休息提醒数据失败") || !strings.Contains(msg.In(err, "en"), "Failed to save break timer data") {
		t.Fatalf("expected bilingual save error, got zh=%q en=%q", err.Error(), msg.In(err, "en"))
	}
	if snap.Today.Water != 0 || e.Snapshot().Today.Water != 0 {
		t.Fatalf("water should roll back on save failure, got %d", e.Snapshot().Today.Water)
	}

	// CompleteBreak fails and rolls back state & breaks count.
	must(e.StartBreakNow())
	snap, err = e.CompleteBreak(true)
	if err == nil {
		t.Fatal("expected CompleteBreak error when breaktimer.json is a directory")
	}
	if snap.State != StateBreak || snap.Today.Breaks != 0 || e.Snapshot().State != StateBreak {
		t.Fatalf("CompleteBreak should roll back on save failure: %+v", e.Snapshot())
	}

	// UpdateConfig fails and rolls back config.
	cfg := DefaultConfig()
	cfg.WorkMinutes = 25
	if err := e.UpdateConfig(cfg); err == nil {
		t.Fatal("expected UpdateConfig error when breaktimer.json is a directory")
	}
	if got := e.Snapshot().Config.WorkMinutes; got != 45 {
		t.Fatalf("config should roll back on save failure, got %d", got)
	}
}

func TestSleepWakeDoesNotFireOverduePopup(t *testing.T) {
	must := mustSnap(t)
	clk := &fakeClock{t: time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)}
	e := New(t.TempDir(), clk.Now)

	must(e.Start())
	e.Tick() // sets lastTick = 10:00:00

	// Computer sleeps for 2 hours, missing the 10:45 break deadline.
	clk.Advance(2 * time.Hour)
	ev := e.Tick()
	if ev.OpenBreak || ev.PlaySound {
		t.Fatalf("sleep/wake must NEVER fire overdue popup or sound: %+v", ev)
	}
	if ev.Snapshot.State != StateWorking || ev.Snapshot.RemainingSec != 45*60 {
		t.Fatalf("sleep/wake while working should silently reset to fresh work round: %+v", ev.Snapshot)
	}

	// Computer sleeps during an active break past break deadline.
	must(e.StartBreakNow())
	e.Tick()
	clk.Advance(30 * time.Minute)
	ev = e.Tick()
	if ev.OpenBreak || ev.PlaySound || !ev.HideBreak {
		t.Fatalf("sleep/wake past break should silently hide without sound/popup: %+v", ev)
	}
	if ev.Snapshot.State != StateIdle {
		t.Fatalf("after missed break without auto-schedule, expected idle, got %s", ev.Snapshot.State)
	}
}

func TestAutoScheduleWeekdaysAndManualStop(t *testing.T) {
	must := mustSnap(t)
	// Start on Saturday 10:00 -> outside_hours when weekdays_only=true
	clk := &fakeClock{t: time.Date(2026, 11, 21, 10, 0, 0, 0, time.Local)} // Saturday
	e := New(t.TempDir(), clk.Now)
	cfg := DefaultConfig()
	cfg.AutoSchedule = true
	if err := e.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if got := e.Snapshot().State; got != StateOutsideHours {
		t.Fatalf("Saturday should be outside_hours, got %s", got)
	}

	// Move to Monday 08:59 -> still outside_hours
	clk.t = time.Date(2026, 11, 23, 8, 59, 0, 0, time.Local)
	if got := e.Tick().Snapshot.State; got != StateOutsideHours {
		t.Fatalf("Monday 08:59 should be outside_hours, got %s", got)
	}

	// Monday 09:00 -> auto starts working
	clk.Advance(time.Minute)
	if got := e.Tick().Snapshot.State; got != StateWorking {
		t.Fatalf("Monday 09:00 should auto start working, got %s", got)
	}

	// Manual stop stays idle for the rest of today's window
	if got := must(e.Stop()).State; got != StateIdle {
		t.Fatalf("manual stop should go idle, got %s", got)
	}
	clk.Advance(time.Second)
	if got := e.Tick().Snapshot.State; got != StateIdle {
		t.Fatalf("manual stop should not be immediately restarted on same day, got %s", got)
	}

	// Next day inside hours -> auto starts again
	clk.t = time.Date(2026, 11, 24, 9, 1, 0, 0, time.Local)
	if got := e.Tick().Snapshot.State; got != StateWorking {
		t.Fatalf("next day in hours should auto start working, got %s", got)
	}

	// Leaving work hours -> outside_hours
	clk.t = time.Date(2026, 11, 24, 18, 30, 0, 0, time.Local)
	if got := e.Tick().Snapshot.State; got != StateOutsideHours {
		t.Fatalf("after 18:30 should go outside_hours, got %s", got)
	}
}

func TestConfigValidationAndBilingualErrors(t *testing.T) {
	e := New(t.TempDir(), nil)
	badCases := []func(*Config){
		func(c *Config) { c.WorkMinutes = 4 },
		func(c *Config) { c.WorkMinutes = 181 },
		func(c *Config) { c.BreakMinutes = 0 },
		func(c *Config) { c.BreakMinutes = 31 },
		func(c *Config) { c.SnoozeMinutes = 0 },
		func(c *Config) { c.WaterGoal = 0 },
		func(c *Config) { c.ScheduleStart = "9:00" },
		func(c *Config) { c.ScheduleEnd = "25:00" },
		func(c *Config) { c.ScheduleStart = "18:30"; c.ScheduleEnd = "09:00" },
	}
	for i, mutate := range badCases {
		cfg := DefaultConfig()
		mutate(&cfg)
		err := e.UpdateConfig(cfg)
		if err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
		if msg.In(err, "en") == err.Error() {
			t.Fatalf("case %d: error missing English translation: %v", i, err)
		}
	}
}

func TestHistoryPrunesOldDays(t *testing.T) {
	must := mustSnap(t)
	clk := &fakeClock{t: time.Date(2026, 11, 21, 10, 0, 0, 0, time.Local)}
	dir := t.TempDir()
	// Pre-seed a 40-day-old entry
	seed := `{"config":{"work_minutes":45,"break_minutes":3,"snooze_minutes":5,"water_goal":8,"auto_schedule":false,"schedule_start":"09:00","schedule_end":"18:30","weekdays_only":true,"popup_enabled":true,"sound_enabled":true,"menu_bar_countdown":true},"days":{"2026-10-01":{"date":"2026-10-01","breaks":3,"water":5}}}`
	if err := os.WriteFile(filepath.Join(dir, "breaktimer.json"), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	e := New(dir, clk.Now)
	must(e.AddWater(1))

	e2 := New(dir, clk.Now)
	if _, ok := e2.days["2026-10-01"]; ok {
		t.Fatalf("old day 2026-10-01 should have been pruned")
	}
	if e2.days["2026-11-21"].Water != 1 {
		t.Fatalf("today water=%d, want 1", e2.days["2026-11-21"].Water)
	}
}
