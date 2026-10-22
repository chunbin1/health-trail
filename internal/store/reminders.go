package store

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Reminder is one notification to schedule: a habit not checked off, a
// medicine not taken, an appointment coming up. Title and Body are Chinese;
// TitleEN and BodyEN are for the English UI (names typed by the person stay
// as they are).
type Reminder struct {
	ID      string
	At      time.Time
	Title   string
	Body    string
	TitleEN string
	BodyEN  string
}

// MaxReminders keeps under macOS's limit of 64 pending notifications per app.
const MaxReminders = 60

const clockFormat = "2006-01-02 15:04"

// Reminders lists the notifications due after now within the next days days,
// soonest first, at most MaxReminders:
//   - each active habit with a remind_at, on each day it isn't checked in yet;
//   - each medicine in use at each of its remind_times, unless a dose was
//     already logged for that time (see doseSlots);
//   - each upcoming appointment, at 20:00 the evening before and its
//     remind_before ahead (1 hour if not set).
//
// The ID carries the time, so a changed time is a new notification.
func (s *Store) Reminders(now time.Time, days int) ([]Reminder, error) {
	var out []Reminder
	for _, f := range []func(time.Time, int) ([]Reminder, error){s.habitReminders, s.medReminders, s.appointmentReminders} {
		rs, err := f(now, days)
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	if len(out) > MaxReminders {
		out = out[:MaxReminders]
	}
	return out, nil
}

func (s *Store) habitReminders(now time.Time, days int) ([]Reminder, error) {
	habits, err := s.query(`SELECT * FROM habits WHERE remind_at IS NOT NULL AND (archived IS NULL OR archived = 0) ORDER BY id`)
	if err != nil {
		return nil, err
	}
	cs, err := s.query(`SELECT habit_id, date FROM checkins WHERE date >= ?`, now.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	done := map[string]bool{}
	for _, c := range cs {
		done[fmt.Sprint(c["habit_id"], " ", c["date"])] = true
	}
	var out []Reminder
	for _, h := range habits {
		at, _ := h["remind_at"].(string)
		clock, err := time.Parse("15:04", at)
		if err != nil {
			continue
		}
		title := strings.TrimSpace(fmt.Sprint(h["name"]))
		if icon, _ := h["icon"].(string); icon != "" {
			title = icon + " " + title
		}
		for d := 0; d < days; d++ {
			t := time.Date(now.Year(), now.Month(), now.Day()+d, clock.Hour(), clock.Minute(), 0, 0, now.Location())
			date := t.Format("2006-01-02")
			if !t.After(now) || done[fmt.Sprint(h["id"], " ", date)] {
				continue
			}
			out = append(out, Reminder{
				ID: fmt.Sprintf("habit-%v-%s-%s", h["id"], date, at), At: t,
				Title: title, Body: "到点了，今天还没打卡",
				TitleEN: title, BodyEN: "Time's up, and it isn't checked off today",
			})
		}
	}
	return out, nil
}

// MedSlot is one time of day a medicine is due, with the dose that covers
// it, if one was logged.
type MedSlot struct {
	Time string    `json:"time"` // HH:MM
	At   time.Time `json:"-"`
	Dose Record    `json:"dose"` // nil if not taken
}

// doseSlots matches doses to the times a medicine is due on day. A dose
// counts for the nearest time: each time owns the half of the gaps on
// either side of it (around midnight too), so with 08:00 and 20:00 a pill
// at 07:30 or 11:00 is the morning's, and one at 19:00 the evening's.
func doseSlots(day time.Time, times []string, doses []Record) []MedSlot {
	var at []time.Time
	for _, t := range times {
		c, err := time.Parse("15:04", t)
		if err != nil {
			continue
		}
		at = append(at, time.Date(day.Year(), day.Month(), day.Day(), c.Hour(), c.Minute(), 0, 0, day.Location()))
	}
	out := make([]MedSlot, len(at))
	for i, a := range at {
		prev, next := a.Add(-24*time.Hour), a.Add(24*time.Hour)
		if i > 0 {
			prev = at[i-1]
		} else if len(at) > 1 {
			prev = at[len(at)-1].Add(-24 * time.Hour)
		}
		if i < len(at)-1 {
			next = at[i+1]
		} else if len(at) > 1 {
			next = at[0].Add(24 * time.Hour)
		}
		from, to := a.Add(-a.Sub(prev)/2), a.Add(next.Sub(a)/2)
		out[i] = MedSlot{Time: a.Format("15:04"), At: a}
		for _, d := range doses {
			t, err := time.ParseInLocation(clockFormat, fmt.Sprint(d["taken_at"]), day.Location())
			if err == nil && t.After(from) && !t.After(to) {
				out[i].Dose = d
			}
		}
	}
	return out
}

// medsDue are the medicines in use on date that have remind_times.
func (s *Store) medsDue(date string) ([]Record, error) {
	return s.query(`SELECT * FROM medications WHERE remind_times IS NOT NULL
		AND (status IS NULL OR status = '在用')
		AND (start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date >= ?)
		ORDER BY id`, date, date)
}

func (s *Store) dosesSince(t time.Time) (map[any][]Record, error) {
	ds, err := s.query(`SELECT * FROM doses WHERE taken_at >= ? ORDER BY taken_at`, t.Format(clockFormat))
	if err != nil {
		return nil, err
	}
	by := map[any][]Record{}
	for _, d := range ds {
		by[d["medication_id"]] = append(by[d["medication_id"]], d)
	}
	return by, nil
}

func medTitle(m Record) string {
	name := strings.TrimSpace(fmt.Sprint(m["name"]))
	if d, _ := m["dosage"].(string); d != "" {
		name += " " + d
	}
	return name
}

func (s *Store) medReminders(now time.Time, days int) ([]Reminder, error) {
	doses, err := s.dosesSince(now.AddDate(0, 0, -2))
	if err != nil {
		return nil, err
	}
	var out []Reminder
	for d := 0; d < days; d++ {
		day := time.Date(now.Year(), now.Month(), now.Day()+d, 0, 0, 0, 0, now.Location())
		meds, err := s.medsDue(day.Format("2006-01-02"))
		if err != nil {
			return nil, err
		}
		for _, m := range meds {
			times, _ := m["remind_times"].(string)
			for _, slot := range doseSlots(day, ParseTimes(times), doses[m["id"]]) {
				if !slot.At.After(now) || slot.Dose != nil {
					continue
				}
				body := slot.Time
				if f, _ := m["frequency"].(string); f != "" {
					body += " · " + f
				}
				out = append(out, Reminder{
					ID: fmt.Sprintf("med-%v-%s", m["id"], slot.At.Format("200601021504")), At: slot.At,
					Title: "💊 该吃药了：" + medTitle(m), Body: body,
					TitleEN: "💊 Time for " + medTitle(m), BodyEN: body,
				})
			}
		}
	}
	return out, nil
}

// MedDay is a medicine due today with its times and the doses taken.
type MedDay struct {
	Medication Record    `json:"medication"`
	Slots      []MedSlot `json:"slots"`
}

// MedsToday lists the medicines with remind_times due today, each time
// with the dose that covers it.
func (s *Store) MedsToday(now time.Time) ([]MedDay, error) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	meds, err := s.medsDue(day.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	doses, err := s.dosesSince(day.AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	out := []MedDay{}
	for _, m := range meds {
		times, _ := m["remind_times"].(string)
		out = append(out, MedDay{Medication: m, Slots: doseSlots(day, ParseTimes(times), doses[m["id"]])})
	}
	return out, nil
}

// MedTime is one time a medicine is due, with the dose that covers it.
type MedTime struct {
	Medication Record
	At         time.Time
	Dose       Record // nil if not taken
}

// MedTimes lists each time a medicine in use is due, from the start of
// from's day for days days, each with the dose that covers it.
func (s *Store) MedTimes(from time.Time, days int) ([]MedTime, error) {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	doses, err := s.dosesSince(start.AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	var out []MedTime
	for d := 0; d < days; d++ {
		day := start.AddDate(0, 0, d)
		meds, err := s.medsDue(day.Format("2006-01-02"))
		if err != nil {
			return nil, err
		}
		for _, m := range meds {
			times, _ := m["remind_times"].(string)
			for _, slot := range doseSlots(day, ParseTimes(times), doses[m["id"]]) {
				out = append(out, MedTime{Medication: m, At: slot.At, Dose: slot.Dose})
			}
		}
	}
	return out, nil
}

// DoseTimeFor is when to log a dose taken for a medicine's due time at: at
// itself if it counts for that time (see doseSlots), else the due time, so
// a late tick doesn't land on the next time instead.
func DoseTimeFor(med Record, due, at time.Time) time.Time {
	times, _ := med["remind_times"].(string)
	day := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, due.Location())
	for _, sl := range doseSlots(day, ParseTimes(times), []Record{{"taken_at": at.Format(clockFormat)}}) {
		if sl.At.Equal(due) && sl.Dose != nil {
			return at
		}
	}
	return due
}

// AppointmentsSince lists the appointments from t on, whatever their status.
func (s *Store) AppointmentsSince(t time.Time) ([]Record, error) {
	return s.query(`SELECT * FROM appointments WHERE scheduled_at >= ? ORDER BY scheduled_at`, t.Format(clockFormat))
}

var remindOffsets = map[string]time.Duration{
	"15 分钟": 15 * time.Minute, "30 分钟": 30 * time.Minute, "1 小时": time.Hour,
	"2 小时": 2 * time.Hour, "1 天": 24 * time.Hour,
}

var offsetWords = map[time.Duration][2]string{
	15 * time.Minute: {"15 分钟后", "in 15 minutes"}, 30 * time.Minute: {"30 分钟后", "in 30 minutes"},
	time.Hour: {"1 小时后", "in 1 hour"}, 2 * time.Hour: {"2 小时后", "in 2 hours"},
}

// pendingAppointments are those not yet done or cancelled.
const pendingAppointments = `(status IS NULL OR status = '待去')`

func (s *Store) appointmentReminders(now time.Time, days int) ([]Reminder, error) {
	end := now.AddDate(0, 0, days+1) // the evening-before reminder of the day after
	as, err := s.query(`SELECT * FROM appointments WHERE `+pendingAppointments+` AND scheduled_at > ? AND scheduled_at < ? ORDER BY scheduled_at`,
		now.Format(clockFormat), end.Format(clockFormat))
	if err != nil {
		return nil, err
	}
	horizon := now.AddDate(0, 0, days)
	var out []Reminder
	for _, a := range as {
		at, err := time.ParseInLocation(clockFormat, fmt.Sprint(a["scheduled_at"]), now.Location())
		if err != nil {
			continue
		}
		rb, _ := a["remind_before"].(string)
		if rb == "不提醒" {
			continue
		}
		before, ok := remindOffsets[rb]
		if !ok {
			before = time.Hour
		}
		title := strings.TrimSpace(fmt.Sprint(a["title"]))
		var details []string
		for _, k := range []string{"place", "preparation"} {
			if v, _ := a[k].(string); strings.TrimSpace(v) != "" {
				details = append(details, strings.TrimSpace(v))
			}
		}
		body, bodyEN := strings.Join(details, " · "), strings.Join(details, " · ")
		if body == "" {
			body, bodyEN = "记得按时去", "Don't forget"
		}
		clock := at.Format("15:04")
		id := fmt.Sprintf("appt-%v-%s", a["id"], at.Format("200601021504"))
		add := func(kind string, when time.Time, t, tEN string) {
			if when.After(now) && when.Before(horizon) {
				out = append(out, Reminder{ID: id + "-" + kind, At: when, Title: "📅 " + t, Body: body, TitleEN: "📅 " + tEN, BodyEN: bodyEN})
			}
		}
		// the evening before, unless that is the reminder asked for anyway
		eve := time.Date(at.Year(), at.Month(), at.Day()-1, 20, 0, 0, 0, at.Location())
		if before != 24*time.Hour {
			add("eve", eve, "明天 "+clock+" "+title, "Tomorrow "+clock+" "+title)
		}
		if w, ok := offsetWords[before]; ok {
			add("before", at.Add(-before), clock+" "+title+"（"+w[0]+"）", clock+" "+title+" ("+w[1]+")")
		} else {
			add("before", at.Add(-before), "明天 "+clock+" "+title, "Tomorrow "+clock+" "+title)
		}
	}
	return out, nil
}

// UpcomingAppointments are the appointments not yet done or cancelled, from
// a week ago on (a past one is still open: what happened can be recorded),
// soonest first.
func (s *Store) UpcomingAppointments(now time.Time) ([]Record, error) {
	return s.query(`SELECT * FROM appointments WHERE `+pendingAppointments+` AND scheduled_at >= ? ORDER BY scheduled_at LIMIT 10`,
		now.AddDate(0, 0, -7).Format("2006-01-02"))
}
