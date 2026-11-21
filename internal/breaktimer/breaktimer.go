// Package breaktimer runs the standalone work/break & hydration countdown
// engine and stores its configuration and daily summary in breaktimer.json.
package breaktimer

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lee/health-trail/internal/msg"
)

// State describes the current phase of the break timer.
type State string

const (
	StateIdle         State = "idle"
	StateWorking      State = "working"
	StateBreak        State = "break"
	StateBreakDone    State = "break_done"
	StatePaused       State = "paused"
	StateOutsideHours State = "outside_hours"
)

// Config holds user preferences for the break and hydration reminder.
type Config struct {
	WorkMinutes      int    `json:"work_minutes"`
	BreakMinutes     int    `json:"break_minutes"`
	SnoozeMinutes    int    `json:"snooze_minutes"`
	WaterGoal        int    `json:"water_goal"`
	AutoSchedule     bool   `json:"auto_schedule"`
	ScheduleStart    string `json:"schedule_start"`
	ScheduleEnd      string `json:"schedule_end"`
	WeekdaysOnly     bool   `json:"weekdays_only"`
	PopupEnabled     bool   `json:"popup_enabled"`
	SoundEnabled     bool   `json:"sound_enabled"`
	MenuBarCountdown bool   `json:"menu_bar_countdown"`
}

// DefaultConfig returns sensible default settings.
func DefaultConfig() Config {
	return Config{
		WorkMinutes:      45,
		BreakMinutes:     3,
		SnoozeMinutes:    5,
		WaterGoal:        8,
		AutoSchedule:     false,
		ScheduleStart:    "09:00",
		ScheduleEnd:      "18:30",
		WeekdaysOnly:     true,
		PopupEnabled:     true,
		SoundEnabled:     true,
		MenuBarCountdown: true,
	}
}

// DayStat records completed breaks and water cups for one local calendar day.
type DayStat struct {
	Date   string `json:"date"`
	Breaks int    `json:"breaks"`
	Water  int    `json:"water"`
}

// Snapshot is the read-only state returned to the UI and menu bar.
type Snapshot struct {
	State        State     `json:"state"`
	RemainingSec int       `json:"remaining_sec"`
	TotalSec     int       `json:"total_sec"`
	Today        DayStat   `json:"today"`
	History      []DayStat `json:"history"`
	Config       Config    `json:"config"`
	BreakSeq     int       `json:"break_seq"`
	ServerNow    int64     `json:"server_now"`
}

// Event accompanies a state change or tick notification.
type Event struct {
	Snapshot  Snapshot
	OpenBreak bool
	HideBreak bool
	PlaySound bool
	NotifyUI  bool
}

type persisted struct {
	Config Config             `json:"config"`
	Days   map[string]DayStat `json:"days,omitempty"`
}

const (
	historyRetainDays = 30
	historyViewDays   = 7
	sleepJumpLimit    = 60 * time.Second
)

// Engine drives the work/break state machine and persists daily stats.
type Engine struct {
	mu                 sync.Mutex
	path               string
	now                func() time.Time
	cfg                Config
	days               map[string]DayStat
	state              State
	deadline           time.Time
	totalSec           int
	pausedFrom         State
	pausedRemainingSec int
	pausedTotalSec     int
	manualStoppedDate  string
	breakSeq           int
	lastTick           time.Time
	lastDate           string
	onChange           func(Event)
}

// New creates an Engine backed by dataDir/breaktimer.json.
func New(dataDir string, now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	e := &Engine{
		now:   now,
		cfg:   DefaultConfig(),
		days:  map[string]DayStat{},
		state: StateIdle,
	}
	if dataDir != "" {
		e.path = filepath.Join(dataDir, "breaktimer.json")
		e.load()
	}
	t := e.currentWallTime()
	e.lastDate = dateStr(t)
	e.applyScheduleLocked(t)
	return e
}

// SetOnChange registers a callback invoked after ticks or state transitions.
// Callbacks are always run with the engine mutex unlocked.
func (e *Engine) SetOnChange(fn func(Event)) {
	e.mu.Lock()
	e.onChange = fn
	e.mu.Unlock()
}

// Snapshot returns a point-in-time copy of the engine state.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked(e.currentWallTime())
}

// Start starts (or restarts) a work countdown round.
func (e *Engine) Start() Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	wasBreak := e.inBreakLocked()
	e.manualStoppedDate = ""
	e.startWorkingLocked(now)
	ev := Event{
		Snapshot:  e.snapshotLocked(now),
		HideBreak: wasBreak,
		NotifyUI:  true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// Pause pauses an active work or break countdown.
func (e *Engine) Pause() Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	if e.state != StateWorking && e.state != StateBreak {
		snap := e.snapshotLocked(now)
		e.mu.Unlock()
		return snap
	}
	rem := remainingSeconds(now, e.deadline)
	if rem < 1 {
		rem = 1
	}
	e.pausedFrom = e.state
	e.pausedRemainingSec = rem
	e.pausedTotalSec = e.totalSec
	e.state = StatePaused
	e.deadline = time.Time{}
	ev := Event{
		Snapshot: e.snapshotLocked(now),
		NotifyUI: true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// Resume continues a paused countdown.
func (e *Engine) Resume() Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	if e.state != StatePaused {
		snap := e.snapshotLocked(now)
		e.mu.Unlock()
		return snap
	}
	e.manualStoppedDate = ""
	target := e.pausedFrom
	if target != StateWorking && target != StateBreak {
		target = StateWorking
	}
	rem := e.pausedRemainingSec
	if rem <= 0 {
		if target == StateBreak {
			rem = e.cfg.BreakMinutes * 60
		} else {
			rem = e.cfg.WorkMinutes * 60
		}
	}
	total := e.pausedTotalSec
	if total < rem {
		total = rem
	}
	e.state = target
	e.totalSec = total
	e.deadline = now.Add(time.Duration(rem) * time.Second)
	e.pausedFrom = ""
	e.pausedRemainingSec = 0
	e.pausedTotalSec = 0
	ev := Event{
		Snapshot: e.snapshotLocked(now),
		NotifyUI: true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// Stop stops the active timer and returns to idle (or outside_hours).
func (e *Engine) Stop() Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	wasBreak := e.inBreakLocked()
	e.clearTimerLocked()
	if e.cfg.AutoSchedule {
		if inSchedule(e.cfg, now) {
			e.manualStoppedDate = dateStr(now)
			e.state = StateIdle
		} else {
			e.state = StateOutsideHours
		}
	} else {
		e.state = StateIdle
	}
	ev := Event{
		Snapshot:  e.snapshotLocked(now),
		HideBreak: wasBreak,
		NotifyUI:  true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// StartBreakNow transitions immediately into a break session.
func (e *Engine) StartBreakNow() Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	e.manualStoppedDate = ""
	e.state = StateBreak
	e.totalSec = e.cfg.BreakMinutes * 60
	e.deadline = now.Add(time.Duration(e.totalSec) * time.Second)
	e.pausedFrom = ""
	e.pausedRemainingSec = 0
	e.pausedTotalSec = 0
	e.breakSeq++
	ev := Event{
		Snapshot:  e.snapshotLocked(now),
		OpenBreak: e.cfg.PopupEnabled,
		NotifyUI:  true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// CompleteBreak marks the current break completed (+1 break count), hides the
// break window, and either starts the next work round or returns to idle.
func (e *Engine) CompleteBreak(startNext bool) Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	if !e.inBreakLocked() {
		snap := e.snapshotLocked(now)
		e.mu.Unlock()
		return snap
	}
	today := dateStr(now)
	st := e.days[today]
	st.Date = today
	st.Breaks++
	e.days[today] = st
	e.pruneAndSaveLocked(now)

	e.clearTimerLocked()
	if startNext {
		e.manualStoppedDate = ""
		if e.cfg.AutoSchedule && !inSchedule(e.cfg, now) {
			e.state = StateOutsideHours
		} else {
			e.startWorkingLocked(now)
		}
	} else {
		if e.cfg.AutoSchedule && inSchedule(e.cfg, now) {
			e.manualStoppedDate = today
			e.state = StateIdle
		} else if e.cfg.AutoSchedule {
			e.state = StateOutsideHours
		} else {
			e.state = StateIdle
		}
	}
	ev := Event{
		Snapshot:  e.snapshotLocked(now),
		HideBreak: true,
		NotifyUI:  true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// Snooze postpones the current break (or extends the current work countdown)
// by Config.SnoozeMinutes.
func (e *Engine) Snooze() Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	if e.state == StateIdle || e.state == StateOutsideHours {
		snap := e.snapshotLocked(now)
		e.mu.Unlock()
		return snap
	}
	snoozeSec := e.cfg.SnoozeMinutes * 60
	snoozeDur := time.Duration(snoozeSec) * time.Second
	wasBreak := e.inBreakLocked()
	e.manualStoppedDate = ""

	switch {
	case wasBreak:
		e.clearTimerLocked()
		e.state = StateWorking
		e.totalSec = snoozeSec
		e.deadline = now.Add(snoozeDur)
	case e.state == StateWorking:
		if now.Before(e.deadline) {
			e.deadline = e.deadline.Add(snoozeDur)
			e.totalSec += snoozeSec
		} else {
			e.deadline = now.Add(snoozeDur)
			e.totalSec = snoozeSec
		}
	case e.state == StatePaused:
		e.pausedRemainingSec += snoozeSec
		e.pausedTotalSec += snoozeSec
	}

	ev := Event{
		Snapshot:  e.snapshotLocked(now),
		HideBreak: wasBreak,
		NotifyUI:  true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// SkipBreak dismisses the current break without counting it and returns to
// a fresh work round (or outside_hours if outside the auto-schedule window).
func (e *Engine) SkipBreak() Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	if !e.inBreakLocked() {
		snap := e.snapshotLocked(now)
		e.mu.Unlock()
		return snap
	}
	e.clearTimerLocked()
	e.manualStoppedDate = ""
	if e.cfg.AutoSchedule && !inSchedule(e.cfg, now) {
		e.state = StateOutsideHours
	} else {
		e.startWorkingLocked(now)
	}
	ev := Event{
		Snapshot:  e.snapshotLocked(now),
		HideBreak: true,
		NotifyUI:  true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// AddWater adjusts today's water cup count by delta (clamped at 0).
func (e *Engine) AddWater(delta int) Snapshot {
	e.mu.Lock()
	now := e.currentWallTime()
	today := dateStr(now)
	st := e.days[today]
	st.Date = today
	st.Water += delta
	if st.Water < 0 {
		st.Water = 0
	}
	e.days[today] = st
	e.pruneAndSaveLocked(now)

	ev := Event{
		Snapshot: e.snapshotLocked(now),
		NotifyUI: true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return ev.Snapshot
}

// UpdateConfig validates, persists, and applies a new Config.
func (e *Engine) UpdateConfig(cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	e.mu.Lock()
	now := e.currentWallTime()
	prev := e.cfg
	e.cfg = cfg

	if cfg.AutoSchedule && (!prev.AutoSchedule || prev.ScheduleStart != cfg.ScheduleStart || prev.ScheduleEnd != cfg.ScheduleEnd || prev.WeekdaysOnly != cfg.WeekdaysOnly) {
		e.manualStoppedDate = ""
	}
	if !cfg.AutoSchedule && e.state == StateOutsideHours {
		e.state = StateIdle
	}
	e.applyScheduleLocked(now)
	e.saveLocked()

	ev := Event{
		Snapshot: e.snapshotLocked(now),
		NotifyUI: true,
	}
	cb := e.onChange
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	return nil
}

// Tick advances the timer by one step, handling transitions, auto-schedule
// boundaries, and sleep/wake jumps without overdue popups.
func (e *Engine) Tick() Event {
	e.mu.Lock()
	now := e.currentWallTime()
	jumped := !e.lastTick.IsZero() && now.Sub(e.lastTick) > sleepJumpLimit
	e.lastTick = now

	var ev Event
	prevState := e.state
	today := dateStr(now)
	dateChanged := e.lastDate != "" && e.lastDate != today
	e.lastDate = today

	if jumped {
		switch e.state {
		case StateWorking:
			if !now.Before(e.deadline) {
				if e.cfg.AutoSchedule && !inSchedule(e.cfg, now) {
					e.clearTimerLocked()
					e.state = StateOutsideHours
				} else {
					e.startWorkingLocked(now)
				}
				ev.NotifyUI = true
			}
		case StateBreak:
			if !now.Before(e.deadline) {
				ev.HideBreak = true
				ev.NotifyUI = true
				e.clearTimerLocked()
				if e.cfg.AutoSchedule {
					if inSchedule(e.cfg, now) {
						e.startWorkingLocked(now)
					} else {
						e.state = StateOutsideHours
					}
				} else {
					e.state = StateIdle
				}
			}
		}
	} else {
		switch e.state {
		case StateWorking:
			if !now.Before(e.deadline) {
				e.state = StateBreak
				e.totalSec = e.cfg.BreakMinutes * 60
				e.deadline = now.Add(time.Duration(e.totalSec) * time.Second)
				e.breakSeq++
				ev.OpenBreak = e.cfg.PopupEnabled
				ev.PlaySound = e.cfg.SoundEnabled
				ev.NotifyUI = true
			}
		case StateBreak:
			if !now.Before(e.deadline) {
				// Stay in break_done: never auto-close, never auto-start next round.
				e.state = StateBreakDone
				e.deadline = time.Time{}
				ev.PlaySound = e.cfg.SoundEnabled
				ev.NotifyUI = true
			}
		}
	}

	e.applyScheduleLocked(now)

	if e.state != prevState || dateChanged {
		ev.NotifyUI = true
	}
	ev.Snapshot = e.snapshotLocked(now)
	shouldEmit := ev.NotifyUI || e.state == StateWorking || e.state == StateBreak
	cb := e.onChange
	e.mu.Unlock()

	if shouldEmit && cb != nil {
		cb(ev)
	}
	return ev
}

// currentWallTime returns the wall-clock time without a monotonic reading so
// sleep/wake jumps on macOS (where monotonic time pauses during sleep) are
// still detected against wall-clock deadlines.
func (e *Engine) currentWallTime() time.Time {
	return e.now().Round(0)
}

func (e *Engine) inBreakLocked() bool {
	return e.state == StateBreak || e.state == StateBreakDone || (e.state == StatePaused && e.pausedFrom == StateBreak)
}

func (e *Engine) startWorkingLocked(now time.Time) {
	e.state = StateWorking
	e.totalSec = e.cfg.WorkMinutes * 60
	e.deadline = now.Add(time.Duration(e.totalSec) * time.Second)
	e.pausedFrom = ""
	e.pausedRemainingSec = 0
	e.pausedTotalSec = 0
}

func (e *Engine) clearTimerLocked() {
	e.deadline = time.Time{}
	e.totalSec = 0
	e.pausedFrom = ""
	e.pausedRemainingSec = 0
	e.pausedTotalSec = 0
}

func (e *Engine) applyScheduleLocked(now time.Time) {
	if !e.cfg.AutoSchedule {
		return
	}
	today := dateStr(now)
	if inSchedule(e.cfg, now) {
		if e.state == StateOutsideHours || (e.state == StateIdle && e.manualStoppedDate != today) {
			e.startWorkingLocked(now)
		}
		return
	}
	if e.state == StateIdle || e.state == StateWorking || e.state == StatePaused && e.pausedFrom != StateBreak {
		e.clearTimerLocked()
		e.state = StateOutsideHours
	}
}

func (e *Engine) snapshotLocked(now time.Time) Snapshot {
	today := dateStr(now)
	todayStat := e.days[today]
	todayStat.Date = today

	history := make([]DayStat, historyViewDays)
	for i := 0; i < historyViewDays; i++ {
		d := dateStr(now.AddDate(0, 0, -(historyViewDays - 1 - i)))
		st := e.days[d]
		st.Date = d
		history[i] = st
	}

	rem := 0
	switch e.state {
	case StateWorking, StateBreak:
		rem = remainingSeconds(now, e.deadline)
	case StatePaused:
		rem = e.pausedRemainingSec
	}

	total := e.totalSec
	if e.state == StatePaused {
		total = e.pausedTotalSec
	}

	return Snapshot{
		State:        e.state,
		RemainingSec: rem,
		TotalSec:     total,
		Today:        todayStat,
		History:      history,
		Config:       e.cfg,
		BreakSeq:     e.breakSeq,
		ServerNow:    now.Unix(),
	}
}

func (e *Engine) load() {
	b, err := os.ReadFile(e.path)
	if err != nil {
		return
	}
	p := persisted{Config: DefaultConfig()}
	if err := json.Unmarshal(b, &p); err != nil {
		return
	}
	if validateConfig(p.Config) == nil {
		e.cfg = p.Config
	}
	for k, v := range p.Days {
		if k != "" && (v.Breaks > 0 || v.Water > 0) {
			v.Date = k
			e.days[k] = v
		}
	}
}

func (e *Engine) pruneAndSaveLocked(now time.Time) {
	cutoff := dateStr(now.AddDate(0, 0, -(historyRetainDays - 1)))
	for k, v := range e.days {
		if k < cutoff || (v.Breaks == 0 && v.Water == 0) {
			delete(e.days, k)
		}
	}
	e.saveLocked()
}

func (e *Engine) saveLocked() {
	if e.path == "" {
		return
	}
	b, err := json.MarshalIndent(persisted{Config: e.cfg, Days: e.days}, "", "  ")
	if err != nil {
		return
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, e.path)
}

func validateConfig(cfg Config) error {
	if cfg.WorkMinutes < 5 || cfg.WorkMinutes > 180 {
		return msg.New("工作时长需在 5 到 180 分钟之间", "Work duration must be between 5 and 180 minutes")
	}
	if cfg.BreakMinutes < 1 || cfg.BreakMinutes > 30 {
		return msg.New("休息时长需在 1 到 30 分钟之间", "Break duration must be between 1 and 30 minutes")
	}
	if cfg.SnoozeMinutes < 1 || cfg.SnoozeMinutes > 30 {
		return msg.New("推迟时长需在 1 到 30 分钟之间", "Snooze duration must be between 1 and 30 minutes")
	}
	if cfg.WaterGoal < 1 || cfg.WaterGoal > 30 {
		return msg.New("喝水目标需在 1 到 30 杯之间", "Daily water goal must be between 1 and 30 cups")
	}
	startMin, okStart := parseHHMM(cfg.ScheduleStart)
	endMin, okEnd := parseHHMM(cfg.ScheduleEnd)
	if !okStart || !okEnd {
		return msg.New("工作时段格式需为 HH:MM", "Schedule time must use HH:MM format")
	}
	if startMin >= endMin {
		return msg.New("工作开始时间要早于结束时间", "Schedule start time must be earlier than end time")
	}
	return nil
}

func parseHHMM(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func inSchedule(cfg Config, t time.Time) bool {
	if cfg.WeekdaysOnly {
		wd := t.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			return false
		}
	}
	startMin, okStart := parseHHMM(cfg.ScheduleStart)
	endMin, okEnd := parseHHMM(cfg.ScheduleEnd)
	if !okStart || !okEnd {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	return cur >= startMin && cur < endMin
}

func remainingSeconds(now, deadline time.Time) int {
	if deadline.IsZero() {
		return 0
	}
	d := deadline.Sub(now)
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(d.Seconds()))
}

func dateStr(t time.Time) string {
	return t.Format("2006-01-02")
}
