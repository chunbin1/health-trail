// Package calsync writes appointments to Calendar and medicines to Reminders.
package calsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lee/health-trail/internal/msg"
	"github.com/lee/health-trail/internal/store"
)

// Appointments go into the Mac's Calendar and medicine times into Reminders,
// each in a calendar/list of the app's own (in iCloud when there is one), so
// they reach the iPhone and the watch and remind there. Ticking a medicine
// off in Reminders logs the dose here; logging it here ticks it off there.
// Each item carries a URL (healthtrail://…) naming the record it comes from:
// healthtrail://appointments/12?uid=…, healthtrail://medications/3/202610250800?uid=….
// The part before "?" is this Mac's id and is how an item is known here
// (its key); the uid names the record on any device. An item written before
// there were uids has no "?uid=" and keeps that URL until it next changes:
// the uid is left out of the digest, so adding it doesn't rewrite anything.

const URLPrefix = "healthtrail://"

// recordURL is the URL for path (appointments/12, …), with the record's uid
// when it has one.
func recordURL(path, uid string) string {
	if uid == "" {
		return URLPrefix + path
	}
	return URLPrefix + path + "?uid=" + uid
}

// keyOf is the URL without the prefix and the uid: how an item is known.
func keyOf(url string) string {
	k := strings.TrimPrefix(url, URLPrefix)
	if i := strings.IndexByte(k, '?'); i >= 0 {
		k = k[:i]
	}
	return k
}

type access int

const (
	accessNotAsked access = iota
	accessFull
	accessDenied
	accessWriteOnly
)

func accessName(a access) msg.Word {
	return [...]msg.Word{{Zh: "还没问过", En: "not asked yet"}, {Zh: "已允许", En: "allowed"}, {Zh: "被拒绝", En: "denied"}, {Zh: "只能写入", En: "write only"}}[a]
}

type calItem struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Completed   bool      `json:"completed"`
	CompletedAt time.Time `json:"-"`
}

type calEvent struct {
	ID       string  `json:"id,omitempty"`
	Title    string  `json:"title"`
	Start    int64   `json:"start"`
	End      int64   `json:"end"`
	Location string  `json:"location"`
	Notes    string  `json:"notes"`
	URL      string  `json:"url"`
	Alarms   []int64 `json:"alarms"`
}

type calReminder struct {
	ID        string  `json:"id,omitempty"`
	Title     string  `json:"title"`
	Due       int64   `json:"due"`
	Notes     string  `json:"notes"`
	URL       string  `json:"url"`
	Completed bool    `json:"completed"`
	Alarms    []int64 `json:"alarms"`
}

// calendarAPI is what the sync needs of Calendar and Reminders: EventKit on
// the Mac, a fake in the tests.
type calendarAPI interface {
	Access(reminders bool) access
	Request(reminders bool) bool
	Calendar(reminders bool, title, saved string) (id, source string, err error)
	Events(calID string, from, to time.Time) ([]calItem, error)
	Reminders(listID string, completedSince time.Time) ([]calItem, error)
	SaveEvent(calID string, e calEvent) (string, error)
	SaveReminder(listID string, r calReminder) (string, error)
	Remove(id string) error
}

type calState struct {
	Events     bool   `json:"events"`    // appointments → Calendar
	Reminders  bool   `json:"reminders"` // medicines → Reminders
	CalendarID string `json:"calendar_id,omitempty"`
	ListID     string `json:"list_id,omitempty"`
	Source     string `json:"source,omitempty"` // the account they are in, e.g. iCloud
	// Made is what this app wrote, by record key: a digest of it, or
	// "gone:" + the digest when the person deleted it there (and it isn't
	// put back until the record changes).
	Made        map[string]string `json:"made,omitempty"`
	LastAt      string            `json:"last_at,omitempty"`
	LastError   string            `json:"last_error,omitempty"`
	LastErrorEn string            `json:"last_error_en,omitempty"`
}

type Sync struct {
	st    *store.Store
	api   calendarAPI
	path  string
	title string // of the calendar and the list
	mu    sync.Mutex
	kick  chan struct{}
}

func New(st *store.Store, api calendarAPI, dataDir, title string) *Sync {
	return &Sync{st: st, api: api, path: filepath.Join(dataDir, "calendar-sync.json"), title: title, kick: make(chan struct{}, 1)}
}

func (c *Sync) load() calState {
	var s calState
	if raw, err := os.ReadFile(c.path); err == nil {
		json.Unmarshal(raw, &s)
	}
	if s.Made == nil {
		s.Made = map[string]string{}
	}
	return s
}

func (c *Sync) save(s calState) error {
	raw, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(c.path, raw, 0o600)
}

// syncing reports which kinds are written to Calendar and Reminders (with
// access), so the app's own notifications for them can step aside instead
// of reminding twice.
func (c *Sync) Syncing() (events, reminders bool) {
	s := c.load()
	return s.Events && c.api.Access(false) == accessFull, s.Reminders && c.api.Access(true) == accessFull
}

func (c *Sync) Loop(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		if err := c.sync(time.Now()); err != nil {
			log.Printf("同步到日历和提醒事项失败：%v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-c.kick:
		}
	}
}

func (c *Sync) poke() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// set turns writing appointments (reminders false) or medicines (true) on
// or off. Turning on asks for access (the system's prompt, the first time)
// and finds or makes the calendar; turning off takes away what is still
// ahead, so nothing stale reminds.
func (c *Sync) Set(reminders, on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.load()
	if on {
		if c.api.Access(reminders) != accessFull && !c.api.Request(reminders) {
			if reminders {
				return msg.New("没有「提醒事项」的权限：系统设置 → 隐私与安全性 → 提醒事项，打开健康足迹", "No access to Reminders: System Settings → Privacy & Security → Reminders, turn on Health Trail")
			}
			return msg.New("没有「日历」的权限：系统设置 → 隐私与安全性 → 日历，打开健康足迹", "No access to Calendars: System Settings → Privacy & Security → Calendars, turn on Health Trail")
		}
		saved := s.CalendarID
		if reminders {
			saved = s.ListID
		}
		id, source, err := c.api.Calendar(reminders, c.title, saved)
		if err != nil {
			return err
		}
		if reminders {
			s.ListID, s.Reminders = id, true
		} else {
			s.CalendarID, s.Events = id, true
		}
		s.Source = source
	} else if c.api.Access(reminders) == accessFull {
		if err := c.clear(&s, reminders, time.Now()); err != nil {
			return err
		}
		s.setOff(reminders)
	} else {
		s.setOff(reminders) // nothing to clear without access
	}
	err := c.save(s)
	c.poke()
	return err
}

func (s *calState) setOff(reminders bool) {
	if reminders {
		s.Reminders = false
	} else {
		s.Events = false
	}
}

// clear removes what the app wrote that is still ahead: the open reminders,
// the events from now on.
func (c *Sync) clear(s *calState, reminders bool, now time.Time) error {
	var items []calItem
	var err error
	if reminders && s.ListID != "" {
		items, err = c.api.Reminders(s.ListID, now)
	} else if !reminders && s.CalendarID != "" {
		items, err = c.api.Events(s.CalendarID, now, now.AddDate(1, 1, 0))
	}
	if err != nil {
		return err
	}
	for _, it := range items {
		if strings.HasPrefix(it.URL, URLPrefix) && !it.Completed {
			if err := c.api.Remove(it.ID); err != nil {
				return err
			}
			delete(s.Made, keyOf(it.URL))
		}
	}
	return nil
}

// sync brings Calendar and Reminders in line with the records, and records
// the medicines ticked off there.
func (c *Sync) sync(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.load()
	if !s.Events && !s.Reminders {
		return nil
	}
	// switched on but no longer allowed (or never quite): say so, don't skip quietly
	var errs []error
	if s.Events {
		if a := c.api.Access(false); a != accessFull {
			errs = append(errs, msg.Errorf("没有「日历」的完整权限（%s）：系统设置 → 隐私与安全性 → 日历", "No full access to Calendars (%s): System Settings → Privacy & Security → Calendars", accessName(a)))
		} else {
			errs = append(errs, c.syncEvents(&s, now))
		}
	}
	if s.Reminders {
		if a := c.api.Access(true); a != accessFull {
			errs = append(errs, msg.Errorf("没有「提醒事项」的完整权限（%s）：系统设置 → 隐私与安全性 → 提醒事项", "No full access to Reminders (%s): System Settings → Privacy & Security → Reminders", accessName(a)))
		} else {
			errs = append(errs, c.syncReminders(&s, now))
		}
	}
	err := msg.Join(errs...)
	s.LastAt, s.LastError, s.LastErrorEn = now.Format("2006-01-02 15:04:05"), "", ""
	if err != nil {
		s.LastError, s.LastErrorEn = err.Error(), msg.In(err, "en")
	}
	if serr := c.save(s); serr != nil && err == nil {
		err = serr
	}
	return err
}

func digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// eventDigest and reminderDigest leave the uid out: what an item was written
// as before uids is the same item now.
func eventDigest(e calEvent) string {
	e.URL = URLPrefix + keyOf(e.URL)
	return digest(e)
}

func reminderDigest(r calReminder) string {
	r.URL = URLPrefix + keyOf(r.URL)
	return digest(r)
}

func byURL(items []calItem) map[string]calItem {
	m := map[string]calItem{}
	for _, it := range items {
		if strings.HasPrefix(it.URL, URLPrefix) {
			m[keyOf(it.URL)] = it
		}
	}
	return m
}

func joinLines(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

func str(r store.Record, k string) string {
	v, _ := r[k].(string)
	return v
}

// appointmentEvent is the event for an appointment: an hour long, with the
// app's reminders as alarms (the evening before and remind_before ahead).
func appointmentEvent(a store.Record, uid string, at time.Time) calEvent {
	e := calEvent{
		Title: str(a, "title"), Start: at.Unix(), End: at.Add(time.Hour).Unix(),
		Location: str(a, "place"), Notes: joinLines(str(a, "preparation"), str(a, "notes")),
		URL: recordURL(fmt.Sprintf("appointments/%v", a["id"]), uid),
	}
	rb := str(a, "remind_before")
	if rb == "不提醒" {
		return e
	}
	before := map[string]time.Duration{"15 分钟": 15 * time.Minute, "30 分钟": 30 * time.Minute, "1 小时": time.Hour, "2 小时": 2 * time.Hour, "1 天": 24 * time.Hour}[rb]
	if before == 0 {
		before = time.Hour
	}
	if before != 24*time.Hour {
		e.Alarms = append(e.Alarms, time.Date(at.Year(), at.Month(), at.Day()-1, 20, 0, 0, 0, at.Location()).Unix())
	}
	e.Alarms = append(e.Alarms, at.Add(-before).Unix())
	return e
}

func (c *Sync) syncEvents(s *calState, now time.Time) error {
	from := now.Add(-24 * time.Hour)
	appts, err := c.st.AppointmentsSince(from)
	if err != nil {
		return err
	}
	existing, err := c.api.Events(s.CalendarID, from, now.AddDate(1, 1, 0))
	if err != nil {
		return err
	}
	have := byURL(existing)
	want := map[string]bool{}
	for _, a := range appts {
		if str(a, "status") == "已取消" {
			continue
		}
		at, err := time.ParseInLocation("2006-01-02 15:04", str(a, "scheduled_at"), now.Location())
		if err != nil {
			continue
		}
		e := appointmentEvent(a, c.uid("appointments", a["id"]), at)
		key := keyOf(e.URL)
		want[key] = true
		h := eventDigest(e)
		ex, ok := have[key]
		prev := s.Made[key]
		switch {
		case ok && prev == h:
			continue
		case !ok && (prev == h || prev == "gone:"+h):
			s.Made[key] = "gone:" + h // deleted there: left alone until the appointment changes
			continue
		case !ok && str(a, "status") == "已完成":
			continue // done before it was ever synced: no event needed now
		}
		if ok {
			e.ID = ex.ID
		}
		if _, err := c.api.SaveEvent(s.CalendarID, e); err != nil {
			return err
		}
		log.Printf("日历：写入 %s（%s）", key, e.Title)
		s.Made[key] = h
	}
	// cancelled or deleted appointments
	for key, ex := range have {
		if !want[key] {
			if err := c.api.Remove(ex.ID); err != nil {
				return err
			}
			delete(s.Made, key)
		}
	}
	forget(s, "appointments/", have, want)
	return nil
}

func (c *Sync) syncReminders(s *calState, now time.Time) error {
	times, err := c.st.MedTimes(now.AddDate(0, 0, -1), 3)
	if err != nil {
		return err
	}
	existing, err := c.api.Reminders(s.ListID, now.Add(-72*time.Hour))
	if err != nil {
		return err
	}
	have := byURL(existing)
	want := map[string]bool{}
	for _, t := range times {
		// from a day back (still to tick off) to the two days ahead
		if t.At.Before(now.Add(-24*time.Hour)) || t.At.After(now.Add(48*time.Hour)) {
			continue
		}
		m := t.Medication
		r := calReminder{
			Title: "💊 " + strings.TrimSpace(str(m, "name")+" "+str(m, "dosage")), Due: t.At.Unix(),
			Notes: str(m, "frequency"), URL: recordURL(fmt.Sprintf("medications/%v/%s", m["id"], t.At.Format("200601021504")), c.uid("medications", m["id"])),
			Completed: t.Dose != nil, Alarms: []int64{t.At.Unix()},
		}
		key := keyOf(r.URL)
		want[key] = true
		open, done := r, r
		open.Completed, done.Completed = false, true
		ex, ok := have[key]
		prev := s.Made[key]
		if ok && ex.Completed && t.Dose == nil && prev != reminderDigest(done) {
			// ticked off on the phone or the Mac: that's a dose taken
			at := ex.CompletedAt
			if at.IsZero() {
				at = now
			}
			at = store.DoseTimeFor(m, t.At, at.In(now.Location()))
			if _, err := c.st.Create("doses", store.Record{"medication_id": m["id"], "taken_at": at.Format("2006-01-02 15:04"), "notes": "在提醒事项里勾选"}); err != nil {
				return err
			}
			s.Made[key] = reminderDigest(done)
			continue
		}
		h := reminderDigest(r)
		switch {
		case ok && prev == h:
			continue
		case !ok && prev != "":
			if !strings.HasPrefix(prev, "gone:") {
				s.Made[key] = "gone:" + prev // deleted there: not put back
			}
			continue
		case !ok && (t.Dose != nil || t.At.Before(now.Add(-2*time.Hour))):
			continue // taken already, or long past: nothing to remind
		}
		if ok {
			r.ID = ex.ID
		}
		if _, err := c.api.SaveReminder(s.ListID, r); err != nil {
			return err
		}
		log.Printf("提醒事项：写入 %s（%s）", key, r.Title)
		s.Made[key] = h
	}
	// a stopped medicine, a changed time, a time gone by: open ones go
	for key, ex := range have {
		if !want[key] && !ex.Completed {
			if err := c.api.Remove(ex.ID); err != nil {
				return err
			}
			delete(s.Made, key)
		}
	}
	forget(s, "medications/", have, want)
	return nil
}

// uid is the record's uid, or "" (written without one) if it can't be read.
func (c *Sync) uid(entity string, id any) string {
	n, _ := id.(int64)
	uid, err := c.st.UID(entity, n)
	if err != nil {
		return ""
	}
	return uid
}

// forget drops the keys of one kind that are neither wanted nor there any
// more, so the state doesn't grow.
func forget(s *calState, kind string, have map[string]calItem, want map[string]bool) {
	for key := range s.Made {
		if _, ok := have[key]; strings.HasPrefix(key, kind) && !ok && !want[key] {
			delete(s.Made, key)
		}
	}
}

// Status is what the settings page shows of the sync: what is on, whether
// macOS allows it, the account, how it last went.
func (c *Sync) Status(lang string) map[string]any {
	s := c.load()
	if lang == "en" && s.LastErrorEn != "" {
		s.LastError = s.LastErrorEn
	}
	names := []string{"not_asked", "full", "denied", "write_only"}
	return map[string]any{
		"events": s.Events, "reminders": s.Reminders, "source": s.Source, "title": c.title,
		"events_access": names[c.api.Access(false)], "reminders_access": names[c.api.Access(true)],
		"last_at": s.LastAt, "last_error": s.LastError,
	}
}
