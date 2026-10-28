package calsync

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lee/health-trail/internal/store"
)

// fakeCal is Calendar and Reminders in memory.
type fakeCal struct {
	access  map[bool]access
	grant   bool // what the permission prompt answers
	nextID  int
	events  map[string]calEvent
	rems    map[string]calReminder
	doneAt  map[string]time.Time
	saves   int
	removes int
}

func newFakeCal() *fakeCal {
	return &fakeCal{access: map[bool]access{}, grant: true, events: map[string]calEvent{}, rems: map[string]calReminder{}, doneAt: map[string]time.Time{}}
}

func (f *fakeCal) Access(r bool) access { return f.access[r] }
func (f *fakeCal) Request(r bool) bool {
	if f.grant {
		f.access[r] = accessFull
	} else {
		f.access[r] = accessDenied
	}
	return f.grant
}
func (f *fakeCal) Calendar(r bool, title, saved string) (string, string, error) {
	if r {
		return "list-" + title, "iCloud", nil
	}
	return "cal-" + title, "iCloud", nil
}
func (f *fakeCal) Events(calID string, from, to time.Time) ([]calItem, error) {
	var out []calItem
	for id, e := range f.events {
		if e.Start >= from.Unix() && e.Start < to.Unix() {
			out = append(out, calItem{ID: id, URL: e.URL})
		}
	}
	return out, nil
}
func (f *fakeCal) Reminders(listID string, since time.Time) ([]calItem, error) {
	var out []calItem
	for id, r := range f.rems {
		if !r.Completed || !f.doneAt[id].Before(since) {
			out = append(out, calItem{ID: id, URL: r.URL, Completed: r.Completed, CompletedAt: f.doneAt[id]})
		}
	}
	return out, nil
}
func (f *fakeCal) SaveEvent(calID string, e calEvent) (string, error) {
	f.saves++
	if e.ID == "" {
		f.nextID++
		e.ID = fmt.Sprint("e", f.nextID)
	}
	f.events[e.ID] = e
	return e.ID, nil
}
func (f *fakeCal) SaveReminder(listID string, r calReminder) (string, error) {
	f.saves++
	if r.ID == "" {
		f.nextID++
		r.ID = fmt.Sprint("r", f.nextID)
	}
	f.rems[r.ID] = r
	return r.ID, nil
}
func (f *fakeCal) Remove(id string) error {
	f.removes++
	delete(f.events, id)
	delete(f.rems, id)
	return nil
}

func (f *fakeCal) remByURL(suffix string) (string, calReminder) {
	for id, r := range f.rems {
		if strings.HasSuffix(keyOf(r.URL), suffix) {
			return id, r
		}
	}
	return "", calReminder{}
}

func calTest(t *testing.T) (*store.Store, *fakeCal, *Sync) {
	t.Helper()
	data := t.TempDir()
	st, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	f := newFakeCal()
	return st, f, New(st, f, data, "健康足迹")
}

func uidOf(t *testing.T, st *store.Store, entity string, id any) string {
	t.Helper()
	n, _ := id.(int64)
	uid, err := st.UID(entity, n)
	if err != nil || !store.ValidUID(uid) {
		t.Fatalf("%s #%v: %q %v", entity, id, uid, err)
	}
	return uid
}

// What was written before there were uids is the same item: not rewritten,
// a deleted one not put back, a ticked-off medicine not taken twice.
func TestItemsFromBeforeUIDs(t *testing.T) {
	st, f, c := calTest(t)
	loc := time.Local
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, loc)
	a, _ := st.Create("appointments", store.Record{"scheduled_at": "2026-10-23 15:00", "title": "胃镜检查"})
	gone, _ := st.Create("appointments", store.Record{"scheduled_at": "2026-10-24 15:00", "title": "删掉的"})
	m, _ := st.Create("medications", store.Record{"name": "奥美拉唑", "status": "在用", "remind_times": "20:00"})
	if err := c.Set(false, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Set(true, true); err != nil {
		t.Fatal(err)
	}
	// as an older version wrote them: URLs without the uid
	s := c.load()
	at := time.Date(2026, 10, 23, 15, 0, 0, 0, loc)
	old := appointmentEvent(a, "", at)
	f.events["e-old"] = calEvent{ID: "e-old", Title: old.Title, Start: old.Start, End: old.End, Notes: old.Notes, URL: old.URL, Alarms: old.Alarms}
	s.Made[keyOf(old.URL)] = digest(old)
	goneEv := appointmentEvent(gone, "", time.Date(2026, 10, 24, 15, 0, 0, 0, loc))
	s.Made[keyOf(goneEv.URL)] = "gone:" + digest(goneEv)
	due := time.Date(2026, 10, 22, 20, 0, 0, 0, loc)
	rem := calReminder{
		Title: "💊 奥美拉唑", Due: due.Unix(), URL: fmt.Sprintf("%smedications/%v/%s", URLPrefix, m["id"], due.Format("200601021504")),
		Completed: true, Alarms: []int64{due.Unix()},
	}
	f.rems["r-old"] = rem
	f.doneAt["r-old"] = due
	s.Made[keyOf(rem.URL)] = digest(rem) // ticked off because the dose was logged here, and that dose since deleted
	if err := c.save(s); err != nil {
		t.Fatal(err)
	}
	f.saves = 0
	if err := c.sync(now); err != nil {
		t.Fatal(err)
	}
	if e := f.events["e-old"]; e.URL != old.URL {
		t.Errorf("unchanged event rewritten: %+v", e)
	}
	for _, e := range f.events {
		if e.Title == "删掉的" {
			t.Errorf("deleted event put back: %+v", e)
		}
	}
	if doses, _, _ := st.List("doses", store.ListQuery{}); len(doses) != 0 {
		t.Errorf("a dose from the old tick: %v", doses)
	}
	// a change writes the uid into the same event
	if _, err := st.Update("appointments", idOf(a), store.Record{"place": "协和医院"}); err != nil {
		t.Fatal(err)
	}
	if err := c.sync(now); err != nil {
		t.Fatal(err)
	}
	if e := f.events["e-old"]; !strings.HasSuffix(e.URL, "?uid="+uidOf(t, st, "appointments", a["id"])) || e.Location != "协和医院" {
		t.Errorf("changed event: %+v", e)
	}
}

func idOf(r store.Record) int64 { n, _ := r["id"].(int64); return n }

func TestCalendarSyncAppointments(t *testing.T) {
	st, f, c := calTest(t)
	loc := time.Local
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, loc)
	a, _ := st.Create("appointments", store.Record{"scheduled_at": "2026-10-23 15:00", "title": "胃镜检查", "place": "协和医院", "preparation": "空腹 8 小时"})
	cancelled, _ := st.Create("appointments", store.Record{"scheduled_at": "2026-10-24 09:00", "title": "取消的", "status": "已取消"})

	// refused access: nothing is turned on
	f.grant = false
	if err := c.Set(false, true); err == nil || !strings.Contains(err.Error(), "日历") {
		t.Fatalf("denied: %v", err)
	}
	if c.load().Events {
		t.Fatal("events on without access")
	}
	f.grant = true
	if err := c.Set(false, true); err != nil {
		t.Fatal(err)
	}
	if err := c.sync(now); err != nil {
		t.Fatal(err)
	}
	if len(f.events) != 1 {
		t.Fatalf("events = %+v", f.events)
	}
	var id string
	var e calEvent
	for id, e = range f.events {
	}
	start := time.Date(2026, 10, 23, 15, 0, 0, 0, loc)
	if e.Title != "胃镜检查" || e.Start != start.Unix() || e.End != start.Add(time.Hour).Unix() || e.Location != "协和医院" ||
		e.Notes != "空腹 8 小时" || e.URL != fmt.Sprintf("healthtrail://appointments/%v?uid=%s", a["id"], uidOf(t, st, "appointments", a["id"])) {
		t.Errorf("event = %+v", e)
	}
	// alarms: 20:00 the evening before, an hour before
	if len(e.Alarms) != 2 || e.Alarms[0] != time.Date(2026, 10, 22, 20, 0, 0, 0, loc).Unix() || e.Alarms[1] != start.Add(-time.Hour).Unix() {
		t.Errorf("alarms = %v", e.Alarms)
	}

	// nothing changed: nothing written
	saves := f.saves
	c.sync(now)
	if f.saves != saves {
		t.Error("an unchanged appointment was written again")
	}
	// changed: the same event is updated
	st.Update("appointments", a["id"].(int64), store.Record{"scheduled_at": "2026-10-23 16:00"})
	c.sync(now)
	if len(f.events) != 1 || f.events[id].Start != start.Add(time.Hour).Unix() {
		t.Errorf("after the move = %+v", f.events)
	}
	// deleted in Calendar: left alone until the appointment changes again
	f.Remove(id)
	c.sync(now)
	if len(f.events) != 0 {
		t.Error("an event deleted in Calendar came back")
	}
	st.Update("appointments", a["id"].(int64), store.Record{"place": "协和医院 内镜中心"})
	c.sync(now)
	if len(f.events) != 1 {
		t.Error("the changed appointment wasn't written again")
	}
	// cancelled: the event goes
	st.Update("appointments", a["id"].(int64), store.Record{"status": "已取消"})
	c.sync(now)
	if len(f.events) != 0 {
		t.Errorf("a cancelled appointment kept its event: %+v", f.events)
	}
	_ = cancelled

	// the app's own notifications step aside while syncing
	if ev, rem := c.Syncing(); !ev || rem {
		t.Errorf("syncing = %v %v", ev, rem)
	}
}

func TestCalendarSyncMedicines(t *testing.T) {
	st, f, c := calTest(t)
	loc := time.Local
	now := time.Date(2026, 10, 22, 7, 0, 0, 0, loc)
	m, _ := st.Create("medications", store.Record{"name": "奥美拉唑", "dosage": "20mg", "frequency": "饭前", "status": "在用", "remind_times": "08:00, 20:00"})
	if err := c.Set(true, true); err != nil {
		t.Fatal(err)
	}
	c.sync(now)
	// yesterday's 20:00 is long past; today's two and tomorrow's two
	if len(f.rems) != 4 {
		t.Fatalf("reminders = %+v", f.rems)
	}
	morning := fmt.Sprintf("medications/%v/202610220800", m["id"])
	id, r := f.remByURL(morning)
	if r.Title != "💊 奥美拉唑 20mg" || r.Due != time.Date(2026, 10, 22, 8, 0, 0, 0, loc).Unix() || r.Notes != "饭前" || r.Completed {
		t.Errorf("morning = %+v", r)
	}

	// ticked off on the phone at 08:05: a dose at 08:05
	r.Completed = true
	f.rems[id] = r
	f.doneAt[id] = time.Date(2026, 10, 22, 8, 5, 0, 0, loc)
	c.sync(now.Add(2 * time.Hour))
	ds, _, _ := st.List("doses", store.ListQuery{})
	if len(ds) != 1 || ds[0]["taken_at"] != "2026-10-22 08:05" || ds[0]["notes"] != "在提醒事项里勾选" {
		t.Fatalf("doses = %v", ds)
	}
	c.sync(now.Add(2 * time.Hour))
	if ds, _, _ = st.List("doses", store.ListQuery{}); len(ds) != 1 {
		t.Errorf("the tick was logged twice: %v", ds)
	}

	// logged in the app: the evening's reminder is ticked off
	st.Create("doses", store.Record{"medication_id": m["id"], "taken_at": "2026-10-22 19:30"})
	c.sync(now.Add(13 * time.Hour))
	if _, ev := f.remByURL(fmt.Sprintf("medications/%v/202610222000", m["id"])); !ev.Completed {
		t.Errorf("evening not ticked off: %+v", ev)
	}

	// ticked late (tomorrow 08:00 at 15:00): the dose is the morning's
	tomorrow := fmt.Sprintf("medications/%v/202610230800", m["id"])
	tid, tr := f.remByURL(tomorrow)
	tr.Completed = true
	f.rems[tid] = tr
	f.doneAt[tid] = time.Date(2026, 10, 23, 15, 0, 0, 0, loc)
	c.sync(time.Date(2026, 10, 23, 15, 1, 0, 0, loc))
	ds, _, _ = st.List("doses", store.ListQuery{})
	if len(ds) != 3 || ds[0]["taken_at"] != "2026-10-23 08:00" {
		t.Errorf("late tick = %v", ds)
	}

	// deleted in Reminders: not put back
	eid, _ := f.remByURL(fmt.Sprintf("medications/%v/202610232000", m["id"]))
	f.Remove(eid)
	c.sync(time.Date(2026, 10, 23, 15, 2, 0, 0, loc))
	if id, _ := f.remByURL(fmt.Sprintf("medications/%v/202610232000", m["id"])); id != "" {
		t.Error("a reminder deleted in Reminders came back")
	}

	// stopped: the open ones go, the ticked ones stay
	st.Update("medications", m["id"].(int64), store.Record{"status": "已停用"})
	c.sync(time.Date(2026, 10, 23, 15, 3, 0, 0, loc))
	for _, r := range f.rems {
		if !r.Completed {
			t.Errorf("an open reminder of a stopped medicine stayed: %+v", r)
		}
	}

	// turned off: what is still open goes
	st.Update("medications", m["id"].(int64), store.Record{"status": "在用"})
	c.sync(time.Date(2026, 10, 23, 15, 4, 0, 0, loc))
	if err := c.Set(true, false); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.rems {
		if !r.Completed {
			t.Errorf("an open reminder stayed after turning off: %+v", r)
		}
	}
	if _, rem := c.Syncing(); rem {
		t.Error("still syncing after turning off")
	}
}
