package store

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func reminderIDs(rs []Reminder) string {
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	return strings.Join(ids, " ")
}

func TestRemindTimes(t *testing.T) {
	for in, want := range map[string]any{
		"8:00, 20:00": "08:00, 20:00", "20:00,8:00": "08:00, 20:00", "8点、20点": "08:00, 20:00",
		"08:00 20:00": "08:00, 20:00", "8 点": "08:00", "7:30；12:00；19:00": "07:30, 12:00, 19:00",
		"8:00，8:00": "08:00", "8点半 和 21点": "08:30, 21:00", "": nil,
	} {
		if got, err := normalizeTimes(in); got != want || err != nil {
			t.Errorf("normalizeTimes(%q) = %v %v, want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"早上8点", "25:00", "8:00, 饭后"} {
		if _, err := normalizeTimes(in); err == nil {
			t.Errorf("normalizeTimes(%q) accepted", in)
		}
	}
	s := open(t)
	m, err := s.Create("medications", Record{"name": "奥美拉唑", "remind_times": []any{"20:00", "8:00"}})
	if err != nil || m["remind_times"] != "08:00, 20:00" {
		t.Errorf("from a list: %v %v", m, err)
	}
	if _, err := s.Create("medications", Record{"name": "坏", "remind_times": "饭后"}); err == nil || !strings.Contains(err.Error(), "提醒时间") {
		t.Errorf("bad remind_times: %v", err)
	}
	// an enum option matches without its space
	a, err := s.Create("appointments", Record{"scheduled_at": "2026-10-23 15:00", "title": "胃镜", "remind_before": "2小时"})
	if err != nil || a["remind_before"] != "2 小时" {
		t.Errorf("remind_before 2小时: %v %v", a, err)
	}
}

func TestMedReminders(t *testing.T) {
	s := open(t)
	loc := time.Local
	now := time.Date(2026, 10, 22, 7, 0, 0, 0, loc)
	ome, _ := s.Create("medications", Record{"name": "奥美拉唑", "dosage": "20mg", "frequency": "饭前", "remind_times": "08:00, 20:00", "status": "在用"})
	// not reminded: as needed, stopped, not started, ended, no times
	s.Create("medications", Record{"name": "布洛芬", "remind_times": "09:00", "status": "按需"})
	s.Create("medications", Record{"name": "停了", "remind_times": "09:00", "status": "已停用"})
	s.Create("medications", Record{"name": "下周开始", "remind_times": "09:00", "start_date": "2026-10-29"})
	s.Create("medications", Record{"name": "昨天停", "remind_times": "09:00", "end_date": "2026-10-21"})
	s.Create("medications", Record{"name": "没时间", "status": "在用"})
	id := ome["id"]

	rs, err := s.Reminders(now, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("med-%v-202610220800 med-%v-202610222000 med-%v-202610230800 med-%v-202610232000", id, id, id, id)
	if got := reminderIDs(rs); got != want {
		t.Fatalf("reminders = %s\nwant %s", got, want)
	}
	if rs[0].Title != "💊 该吃药了：奥美拉唑 20mg" || rs[0].Body != "08:00 · 饭前" || rs[0].TitleEN != "💊 Time for 奥美拉唑 20mg" {
		t.Errorf("first = %+v", rs[0])
	}

	// taken early, at 07:30: the morning's reminder goes, the evening's stays
	s.Create("doses", Record{"medication_id": id, "taken_at": "2026-10-22 07:30"})
	rs, _ = s.Reminders(now, 1)
	if got := reminderIDs(rs); got != fmt.Sprintf("med-%v-202610222000", id) {
		t.Errorf("after the morning dose = %s", got)
	}
	// a dose at 19:00 is the evening's, not a second morning one
	s.Create("doses", Record{"medication_id": id, "taken_at": "2026-10-22 19:00"})
	if rs, _ = s.Reminders(now, 1); len(rs) != 0 {
		t.Errorf("after both doses = %s", reminderIDs(rs))
	}

	days, err := s.MedsToday(now)
	if err != nil || len(days) != 1 || len(days[0].Slots) != 2 {
		t.Fatalf("today = %+v %v", days, err)
	}
	if sl := days[0].Slots; sl[0].Time != "08:00" || sl[0].Dose["taken_at"] != "2026-10-22 07:30" || sl[1].Dose["taken_at"] != "2026-10-22 19:00" {
		t.Errorf("slots = %+v", sl)
	}
}

func TestDoseSlots(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	dose := func(at string) Record { return Record{"taken_at": at} }
	cases := []struct {
		times []string
		dose  string
		want  int // the slot the dose covers, -1 for none
	}{
		{[]string{"08:00", "20:00"}, "2026-10-22 13:59", 0},
		{[]string{"08:00", "20:00"}, "2026-10-22 14:01", 1},
		{[]string{"08:00", "20:00"}, "2026-10-22 02:01", 0},  // after the midpoint with last night's 20:00
		{[]string{"08:00", "20:00"}, "2026-10-22 01:59", -1}, // that was last night's
		{[]string{"08:00"}, "2026-10-21 20:30", 0},           // one a day: twelve hours either side
		{[]string{"08:00"}, "2026-10-21 19:30", -1},
		{[]string{"08:00", "12:00", "18:00"}, "2026-10-22 15:30", 2},
	}
	for _, c := range cases {
		got := -1
		for i, sl := range doseSlots(day, c.times, []Record{dose(c.dose)}) {
			if sl.Dose != nil {
				got = i
			}
		}
		if got != c.want {
			t.Errorf("%v with a dose at %s: slot %d, want %d", c.times, c.dose, got, c.want)
		}
	}
}

func TestAppointmentReminders(t *testing.T) {
	s := open(t)
	loc := time.Local
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, loc)
	gastro, err := s.Create("appointments", Record{"scheduled_at": "2026-10-23 15:00", "title": "胃镜检查", "place": "协和医院 消化内镜中心", "preparation": "空腹 8 小时"})
	if err != nil || gastro["scheduled_at"] != "2026-10-23 15:00" || gastro["status"] != "待去" {
		t.Fatalf("appointment = %v %v", gastro, err)
	}
	today, _ := s.Create("appointments", Record{"scheduled_at": "2026-10-22 16:00", "title": "复诊", "remind_before": "30 分钟"})
	dayAhead, _ := s.Create("appointments", Record{"scheduled_at": "2026-10-24 09:00", "title": "体检", "remind_before": "1 天"})
	s.Create("appointments", Record{"scheduled_at": "2026-10-23 10:00", "title": "不用提醒", "remind_before": "不提醒"})
	s.Create("appointments", Record{"scheduled_at": "2026-10-23 10:00", "title": "取消了", "status": "已取消"})
	s.Create("appointments", Record{"scheduled_at": "2026-10-22 11:00", "title": "已经过了"})

	rs, err := s.Reminders(now, 3)
	if err != nil {
		t.Fatal(err)
	}
	g, d, y := gastro["id"], today["id"], dayAhead["id"]
	want := fmt.Sprintf("appt-%v-202610221600-before appt-%v-202610231500-eve appt-%v-202610240900-before appt-%v-202610231500-before",
		d, g, y, g)
	if got := reminderIDs(rs); got != want {
		t.Fatalf("reminders = %s\nwant %s", got, want)
	}
	if r := rs[0]; r.Title != "📅 16:00 复诊（30 分钟后）" || r.Body != "记得按时去" || r.TitleEN != "📅 16:00 复诊 (in 30 minutes)" ||
		!r.At.Equal(time.Date(2026, 10, 22, 15, 30, 0, 0, loc)) {
		t.Errorf("today's = %+v", r)
	}
	if r := rs[1]; r.Title != "📅 明天 15:00 胃镜检查" || r.Body != "协和医院 消化内镜中心 · 空腹 8 小时" ||
		!r.At.Equal(time.Date(2026, 10, 22, 20, 0, 0, 0, loc)) {
		t.Errorf("evening before = %+v", r)
	}
	if r := rs[2]; r.Title != "📅 明天 09:00 体检" || !r.At.Equal(time.Date(2026, 10, 23, 9, 0, 0, 0, loc)) {
		t.Errorf("a day ahead = %+v", r)
	}

	// done: no more reminders; a moved time is a new reminder
	s.Update("appointments", d.(int64), Record{"status": "已完成"})
	s.Update("appointments", g.(int64), Record{"scheduled_at": "2026-10-23 16:00"})
	rs, _ = s.Reminders(now, 3)
	if got := reminderIDs(rs); strings.Contains(got, fmt.Sprintf("appt-%v-", d)) || !strings.Contains(got, fmt.Sprintf("appt-%v-202610231600-before", g)) {
		t.Errorf("after changes = %s", got)
	}

	up, err := s.UpcomingAppointments(now)
	if err != nil || len(up) != 4 || up[0]["title"] != "已经过了" {
		t.Errorf("upcoming = %v %v", up, err)
	}
}
