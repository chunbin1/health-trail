package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLabFlagsAndRanges(t *testing.T) {
	s := open(t)
	rep, err := s.Create("reports", Record{"report_date": "2026/10/15", "title": "血常规+CRP", "report_type": "化验"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in       Record
		flag     string
		low, hig any
	}{
		// printed arrow, kept
		{Record{"item_name": "白细胞", "value_text": "17.62↑", "ref_range": "3.5-9.5"}, "偏高", 3.5, 9.5},
		// no arrow: derived from the range
		{Record{"item_name": "淋巴细胞比例", "value_num": 14.1, "ref_range": "20-50"}, "偏低", 20.0, 50.0},
		{Record{"item_name": "血红蛋白", "value_num": "153", "ref_range": "130～175"}, "正常", nil, nil},
		{Record{"item_name": "尿蛋白", "value_text": "阴性", "ref_range": "阴性"}, "", nil, nil},
		{Record{"item_name": "肌酐", "value_num": 120, "ref_range": "<97"}, "偏高", nil, 97.0},
	}
	for _, c := range cases {
		c.in["report_id"] = rep["id"]
		c.in["test_date"] = rep["report_date"]
		r, err := s.Create("lab_results", c.in)
		if err != nil {
			t.Fatalf("%v: %v", c.in, err)
		}
		if got, _ := r["flag"].(string); got != c.flag {
			t.Errorf("%s flag = %q, want %q", c.in["item_name"], got, c.flag)
		}
		if c.low != nil && r["ref_low"] != c.low || c.hig != nil && r["ref_high"] != c.hig {
			t.Errorf("%s range = %v..%v", c.in["item_name"], r["ref_low"], r["ref_high"])
		}
	}
	// a corrected value re-derives the flag
	rows, _, _ := s.List("lab_results", ListQuery{Filters: map[string]any{"item_name": "白细胞"}})
	r, err := s.Update("lab_results", rows[0]["id"].(int64), Record{"value_num": 6.1})
	if err != nil {
		t.Fatal(err)
	}
	if r["flag"] != "正常" {
		t.Errorf("after correction flag = %v, want 正常", r["flag"])
	}
	// deleting the report deletes its results
	if err := s.Delete("reports", rep["id"].(int64)); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := s.List("lab_results", ListQuery{}); n != 0 {
		t.Errorf("%d lab results left after their report was deleted", n)
	}
}

func TestValidation(t *testing.T) {
	s := open(t)
	var ve *ValidationError
	_, err := s.Create("visits", Record{"hospital": "协和"})
	if !errors.As(err, &ve) || !strings.Contains(err.Error(), "就诊日期") {
		t.Errorf("missing required: %v", err)
	}
	_, err = s.Create("visits", Record{"visit_date": "2026-02-30"})
	if !errors.As(err, &ve) {
		t.Errorf("impossible date accepted: %v", err)
	}
	_, err = s.Create("visits", Record{"visit_date": "2026-01-02", "colour": "red"})
	if !errors.As(err, &ve) || !strings.Contains(err.Error(), "visit_date") {
		t.Errorf("unknown field: %v", err)
	}
	_, err = s.Create("visits", Record{"visit_date": "2026-01-02", "condition_id": 99})
	if !errors.As(err, &ve) {
		t.Errorf("dangling ref accepted: %v", err)
	}
	_, err = s.Create("conditions", Record{"name": "胃炎", "status": "好了"})
	if !errors.As(err, &ve) || !strings.Contains(err.Error(), "已痊愈") {
		t.Errorf("bad enum: %v", err)
	}
	v, err := s.Create("visits", Record{"visit_date": "2026年3月5日"})
	if err != nil || v["visit_date"] != "2026-03-05" {
		t.Errorf("chinese date: %v %v", v, err)
	}
	m, err := s.Create("metrics", Record{"measured_at": "2026-03-05T07:30:00", "metric_type": "血压", "value": 120, "value2": 80})
	if err != nil || m["measured_at"] != "2026-03-05 07:30" || m["unit"] != "mmHg" {
		t.Errorf("metric: %v %v", m, err)
	}
}

// pastCheckin writes a check-in on another day straight into the table, past
// the today-only rule, as the history a test builds on.
func pastCheckin(t *testing.T, s *Store, habitID int64, date string, value float64) int64 {
	t.Helper()
	e, _ := entity("checkins")
	r, err := s.build(e, Record{"habit_id": habitID, "date": date, "value": value})
	if err != nil {
		t.Fatal(err)
	}
	id, err := insert(s.DB, e, r)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCheckinsAndStreak(t *testing.T) {
	s := open(t)
	h, err := s.Create("habits", Record{"name": "跑步", "unit": "公里"})
	if err != nil {
		t.Fatal(err)
	}
	id := h["id"].(int64)
	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }
	yesterday := pastCheckin(t, s, id, day(-1), 10)
	for _, n := range []int{-2, -3, -5} {
		pastCheckin(t, s, id, day(n), 5)
	}
	st, err := s.CheckinStats(day(-6), day(0), id)
	if err != nil {
		t.Fatal(err)
	}
	// today isn't done yet, so the streak runs back from yesterday
	if st[0].DaysDone != 4 || st[0].CurrentStreak != 3 || st[0].LongestStreak != 3 || *st[0].Total != 25 {
		t.Errorf("stats = %+v", st[0])
	}
	five, ten := 5.0, 10.0
	if _, err := s.Checkin(id, day(0), &five, ""); err != nil {
		t.Fatal(err)
	}
	// checking in twice on a day updates it
	if c, err := s.Checkin(id, "today", &ten, "加量"); err != nil || c["value"] != 10.0 || c["notes"] != "加量" {
		t.Fatalf("second check-in today = %v %v", c, err)
	}
	if st, _ := s.CheckinStats("", "", id); st[0].CurrentStreak != 4 || st[0].DaysDone != 5 {
		t.Errorf("stats with today = %+v, want streak 4 over 5 days", st[0])
	}

	// only today's check-in can change: not a past day's, not a future one
	var ve *ValidationError
	if _, err := s.Checkin(id, day(-4), nil, ""); !errors.As(err, &ve) || !strings.Contains(err.Error(), "只能记在今天") {
		t.Errorf("check-in on a past day = %v", err)
	}
	if _, err := s.Checkin(id, day(1), nil, ""); !errors.As(err, &ve) {
		t.Errorf("check-in ahead = %v", err)
	}
	if ok, err := s.Uncheck(id, day(-1)); ok || !errors.As(err, &ve) {
		t.Errorf("uncheck yesterday = %v %v", ok, err)
	}
	if _, err := s.Update("checkins", yesterday, Record{"value": 1}); !errors.As(err, &ve) {
		t.Errorf("editing yesterday = %v", err)
	}
	if _, err := s.Update("checkins", yesterday, Record{"date": day(0)}); !errors.As(err, &ve) {
		t.Errorf("moving yesterday to today = %v", err)
	}
	if err := s.Delete("checkins", yesterday); !errors.As(err, &ve) {
		t.Errorf("deleting yesterday = %v", err)
	}
	if _, err := s.Create("checkins", Record{"habit_id": id, "date": day(-4)}); !errors.As(err, &ve) {
		t.Errorf("creating a past check-in = %v", err)
	}
	if c, _ := s.Get("checkins", yesterday); c["value"] != 10.0 {
		t.Errorf("yesterday changed: %v", c)
	}

	if ok, _ := s.Uncheck(id, day(0)); !ok {
		t.Error("uncheck found nothing")
	}
	if _, err := s.FindHabit("跑"); err != nil {
		t.Errorf("partial habit name: %v", err)
	}
}

func TestCheckinLogsMetric(t *testing.T) {
	s := open(t)
	h, err := s.Create("habits", Record{"name": "晨起称体重", "metric_type": "体重"})
	if err != nil {
		t.Fatal(err)
	}
	if h["unit"] != "kg" {
		t.Errorf("habit unit = %v, want kg from the metric", h["unit"])
	}
	id := h["id"].(int64)
	w := 65.2
	c, err := s.Checkin(id, "today", &w, "")
	if err != nil {
		t.Fatal(err)
	}
	mid, _ := c["metric_id"].(int64)
	m, err := s.Get("metrics", mid)
	if err != nil || m["metric_type"] != "体重" || m["value"] != 65.2 || m["unit"] != "kg" || m["context"] != "晨起称体重" {
		t.Fatalf("metric = %v %v", m, err)
	}
	w = 65.0
	if c, err = s.Checkin(id, "today", &w, ""); err != nil || c["metric_id"] != mid {
		t.Fatalf("second check-in = %v %v", c, err)
	}
	if rows, n, _ := s.List("metrics", ListQuery{}); n != 1 || rows[0]["value"] != 65.0 {
		t.Errorf("metrics after update = %v", rows)
	}
	if st, _ := s.CheckinStats("", "", id); st[0].Total != nil {
		t.Errorf("readings were summed: %v", *st[0].Total)
	}
	if ok, err := s.Uncheck(id, "today"); !ok || err != nil {
		t.Fatalf("uncheck = %v %v", ok, err)
	}
	if _, n, _ := s.List("metrics", ListQuery{}); n != 0 {
		t.Errorf("metric left after uncheck: %d", n)
	}
	// no value: a plain check-in, no metric
	if c, err = s.Checkin(id, "today", nil, ""); err != nil || c["metric_id"] != nil {
		t.Errorf("check-in without value = %v %v", c, err)
	}
	// a refused past check-in leaves the metric of that day alone
	past := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	pm, _ := s.Create("metrics", Record{"measured_at": past, "metric_type": "体重", "value": 66})
	pc := pastCheckin(t, s, id, past, 66)
	// link them the way a check-in with a reading would have
	if _, err := s.DB.Exec(`UPDATE checkins SET metric_id = ? WHERE id = ?`, pm["id"], pc); err != nil {
		t.Fatal(err)
	}
	w = 50
	if _, err := s.Checkin(id, past, &w, ""); err == nil {
		t.Error("past check-in with a reading accepted")
	}
	if m2, _ := s.Get("metrics", pm["id"].(int64)); m2["value"] != 66.0 {
		t.Errorf("refused check-in changed the metric: %v", m2["value"])
	}
	if _, err := s.Create("habits", Record{"name": "量血压", "metric_type": "血压"}); err == nil {
		t.Error("血压 needs two values and should not be a habit metric")
	}
}

// A lab result named like a metric shows in that metric's series and as its
// latest value, without being copied; one in another unit stays out.
func TestMetricSeriesWithLabs(t *testing.T) {
	s := open(t)
	if _, err := s.Create("metric_kinds", Record{"name": "尿酸", "unit": "μmol/L"}); err != nil {
		t.Fatal(err)
	}
	home, _ := s.Create("metrics", Record{"measured_at": "2026-10-21 17:13", "metric_type": "尿酸", "value": 490})
	lab, err := s.Create("lab_results", Record{"test_date": "2026-10-25", "item_name": "尿酸", "value_num": 576, "unit": "µmol/L", "ref_range": "208-428"})
	if err != nil {
		t.Fatal(err)
	}
	s.Create("lab_results", Record{"test_date": "2026-09-21", "item_name": "尿酸", "value_num": 10.3, "unit": "mg/dL"})
	// no unit: could be either, so left out
	s.Create("lab_results", Record{"test_date": "2026-09-22", "item_name": "尿酸", "value_num": 9.8})
	s.Create("lab_results", Record{"test_date": "2026-09-21", "item_name": "尿尿酸", "value_num": 2361, "unit": "µmol/L"})

	pts, err := s.MetricSeries("尿酸", "", "")
	if err != nil || len(pts) != 2 {
		t.Fatalf("series = %v %v, want the home reading and the μmol/L lab result", pts, err)
	}
	if pts[0]["source"] != "metrics" || pts[0]["id"] != home["id"] || pts[1]["source"] != "lab_results" ||
		pts[1]["id"] != lab["id"] || pts[1]["value"] != 576.0 || pts[1]["measured_at"] != "2026-10-25" || pts[1]["flag"] != "偏高" {
		t.Errorf("series = %v", pts)
	}
	// any date the API takes, on both kinds the same way
	for _, from := range []string{"2026-10-22", "2026/10/22", "20261022", "2026-10-22 08:00"} {
		if pts, err := s.MetricSeries("尿酸", from, ""); err != nil || len(pts) != 1 || pts[0]["source"] != "lab_results" {
			t.Errorf("series from %s = %v %v, want only the lab result", from, pts, err)
		}
	}
	for _, to := range []string{"2026-10-24", "2026/10/24"} {
		if pts, err := s.MetricSeries("尿酸", "", to); err != nil || len(pts) != 1 || pts[0]["source"] != "metrics" {
			t.Errorf("series to %s = %v %v, want only the home reading", to, pts, err)
		}
	}
	if _, err := s.MetricSeries("尿酸", "下周", ""); err == nil || !strings.Contains(err.Error(), "from") {
		t.Errorf("bad from: err = %v", err)
	}
	o, err := s.Overview()
	if err != nil || len(o.LatestMetrics) != 1 || o.LatestMetrics[0]["value"] != 576.0 {
		t.Errorf("latest metrics = %v %v, want the lab result", o.LatestMetrics, err)
	}
	if o.Counts["metrics"] != 1 {
		t.Errorf("metrics = %d, the lab result was copied", o.Counts["metrics"])
	}
}

// With more readings than a series holds, the latest ones are kept, and the
// 今天 page still shows the latest value.
func TestMetricSeriesKeepsLatest(t *testing.T) {
	s := open(t)
	day := time.Date(2023, 1, 1, 8, 0, 0, 0, time.Local)
	var in []Record
	for i := 0; i < 1005; i++ {
		in = append(in, Record{"measured_at": day.AddDate(0, 0, i).Format("2006-01-02 15:04"), "metric_type": "体重", "value": 60 + i%30})
	}
	if _, err := s.CreateMany("metrics", in); err != nil {
		t.Fatal(err)
	}
	last := day.AddDate(0, 0, 1004).Format("2006-01-02 15:04")
	pts, err := s.MetricSeries("体重", "", "")
	if err != nil || len(pts) != 1000 || pts[len(pts)-1]["measured_at"] != last || pts[0]["measured_at"] != day.AddDate(0, 0, 5).Format("2006-01-02 15:04") {
		t.Fatalf("series = %d readings, %v … %v, %v; want the latest 1000 ending %s", len(pts), pts[0]["measured_at"], pts[len(pts)-1]["measured_at"], err, last)
	}
	o, err := s.Overview()
	if err != nil || len(o.LatestMetrics) != 1 || o.LatestMetrics[0]["measured_at"] != last {
		t.Errorf("latest = %v %v, want the reading of %s", o.LatestMetrics, err, last)
	}

	// exactly as many as a series holds: nothing cut, so an older lab result shows
	var bg []Record
	for i := 0; i < 1000; i++ {
		bg = append(bg, Record{"measured_at": day.AddDate(0, 0, i).Format("2006-01-02 15:04"), "metric_type": "血糖", "value": 5})
	}
	if _, err := s.CreateMany("metrics", bg); err != nil {
		t.Fatal(err)
	}
	s.Create("lab_results", Record{"test_date": "2022-06-01", "item_name": "血糖", "value_num": 6.1, "unit": "mmol/L"})
	if pts, _ := s.MetricSeries("血糖", "", ""); len(pts) != 1001 || pts[0]["source"] != "lab_results" {
		t.Errorf("series of 1000 readings = %d, first %v; want the 2022 lab result too", len(pts), pts[0])
	}
}

// Of two lab results on one day, the later is the latest, on the 今天 page
// as in the series.
func TestLatestOfTwoLabsOnADay(t *testing.T) {
	s := open(t)
	s.Create("lab_results", Record{"test_date": "2026-10-25", "item_name": "血糖", "value_num": 5.1, "unit": "mmol/L"})
	s.Create("lab_results", Record{"test_date": "2026-10-25", "item_name": "血糖", "value_num": 7.7, "unit": "mmol/L"})
	pts, _ := s.MetricSeries("血糖", "", "")
	o, err := s.Overview()
	if err != nil || len(pts) != 2 || len(o.LatestMetrics) != 1 || o.LatestMetrics[0]["value"] != pts[1]["value"] || pts[1]["value"] != 7.7 {
		t.Errorf("latest = %v, series = %v %v; want 7.7 in both", o.LatestMetrics, pts, err)
	}
}

func TestCustomMetric(t *testing.T) {
	s := open(t)
	if _, err := s.Create("metrics", Record{"measured_at": "today", "metric_type": "尿酸", "value": 420}); err == nil ||
		!strings.Contains(err.Error(), "metric_kinds") {
		t.Fatalf("unknown metric: err = %v, want one pointing to metric_kinds", err)
	}
	if _, err := s.Create("metric_kinds", Record{"name": "体重", "unit": "斤"}); err == nil {
		t.Error("a custom metric named like a built-in one was accepted")
	}
	k, err := s.Create("metric_kinds", Record{"name": "尿酸", "unit": "μmol/L"})
	if err != nil {
		t.Fatal(err)
	}
	kid := k["id"].(int64)
	m, err := s.Create("metrics", Record{"measured_at": "today", "metric_type": "尿酸", "value": 420})
	if err != nil || m["unit"] != "μmol/L" {
		t.Fatalf("custom metric = %v %v, want its unit", m, err)
	}
	mid := m["id"].(int64)
	if pts, err := s.MetricSeries("尿酸", "", ""); err != nil || len(pts) != 1 {
		t.Fatalf("series of a custom metric = %v %v", pts, err)
	}
	// a reading that gave its own unit keeps it when the metric's changes
	own, _ := s.Create("metrics", Record{"measured_at": "today", "metric_type": "尿酸", "value": 7, "unit": "mg/dL"})
	h, err := s.Create("habits", Record{"name": "测尿酸", "metric_type": "尿酸"})
	if err != nil || h["unit"] != "μmol/L" {
		t.Fatalf("habit = %v %v", h, err)
	}

	// another metric is in its own unit
	if m, err = s.Update("metrics", mid, Record{"metric_type": "体重"}); err != nil || m["unit"] != "kg" {
		t.Fatalf("switched to 体重 = %v %v", m, err)
	}
	if m, err = s.Update("metrics", mid, Record{"metric_type": "尿酸"}); err != nil || m["unit"] != "μmol/L" {
		t.Fatalf("switched back = %v %v", m, err)
	}

	// renaming or re-uniting it carries over to what uses it
	if _, err := s.Update("metric_kinds", kid, Record{"name": "血尿酸", "unit": "umol/L"}); err != nil {
		t.Fatal(err)
	}
	if m, _ = s.Get("metrics", mid); m["metric_type"] != "血尿酸" || m["unit"] != "umol/L" {
		t.Errorf("reading after rename = %v %v", m["metric_type"], m["unit"])
	}
	if o, _ := s.Get("metrics", own["id"].(int64)); o["metric_type"] != "血尿酸" || o["unit"] != "mg/dL" {
		t.Errorf("reading with its own unit after rename = %v %v", o["metric_type"], o["unit"])
	}
	if h, _ = s.Get("habits", h["id"].(int64)); h["metric_type"] != "血尿酸" || h["unit"] != "umol/L" {
		t.Errorf("habit after rename = %v %v", h["metric_type"], h["unit"])
	}

	// not deleted while in use
	if err := s.Delete("metric_kinds", kid); err == nil || !strings.Contains(err.Error(), "还有 2 条身体数据和 1 个打卡项在用") {
		t.Fatalf("delete in use: err = %v", err)
	}
	s.Delete("metrics", mid)
	s.Delete("metrics", own["id"].(int64))
	// a check-in item isn't deleted; it stops logging the metric instead
	if _, err := s.Update("habits", h["id"].(int64), Record{"metric_type": ""}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("metric_kinds", kid); err != nil {
		t.Fatalf("delete unused: %v", err)
	}
	o, err := s.Overview()
	if err != nil || len(o.MetricKinds) != 0 || o.Counts["metric_kinds"] != 0 {
		t.Errorf("overview metric_kinds = %v %v", o.MetricKinds, err)
	}
}

func TestHabitReminders(t *testing.T) {
	s := open(t)
	for in, want := range map[string]string{"23:00": "23:00", "23：00": "23:00", " 9:5 ": "09:05", "23": "23:00", "23点": "23:00", "23点30分": "23:30", "11点半": "11:30"} {
		if got, err := normalizeTime(in); got != want || err != nil {
			t.Errorf("normalizeTime(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"24:00", "23:60", "晚上", "23:00:00"} {
		if _, err := normalizeTime(in); err == nil {
			t.Errorf("normalizeTime(%q) accepted", in)
		}
	}
	if _, err := s.Create("habits", Record{"name": "坏", "remind_at": "25点"}); err == nil {
		t.Error("remind_at 25点 accepted")
	}

	sleep, err := s.Create("habits", Record{"name": "作息守住", "icon": "⏰", "remind_at": "23：00"})
	if err != nil || sleep["remind_at"] != "23:00" {
		t.Fatalf("habit = %v %v", sleep, err)
	}
	walk, _ := s.Create("habits", Record{"name": "散步", "remind_at": "08:00"})
	s.Create("habits", Record{"name": "不提醒"})
	s.Create("habits", Record{"name": "已归档", "remind_at": "09:00", "archived": true})

	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 10, 19, 21, 0, 0, 0, loc)
	rs, err := s.Reminders(now, 3)
	if err != nil {
		t.Fatal(err)
	}
	// 散步 today at 08:00 has passed; 作息 tonight is still to come
	want := []string{
		fmt.Sprintf("habit-%v-2026-10-19-23:00", sleep["id"]),
		fmt.Sprintf("habit-%v-2026-10-20-08:00", walk["id"]),
		fmt.Sprintf("habit-%v-2026-10-20-23:00", sleep["id"]),
		fmt.Sprintf("habit-%v-2026-10-21-08:00", walk["id"]),
		fmt.Sprintf("habit-%v-2026-10-21-23:00", sleep["id"]),
	}
	var got []string
	for _, r := range rs {
		got = append(got, r.ID)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("reminders =\n%v\nwant\n%v", got, want)
	}
	if rs[0].Title != "⏰ 作息守住" || !rs[0].At.Equal(time.Date(2026, 10, 19, 23, 0, 0, 0, loc)) {
		t.Errorf("first = %+v", rs[0])
	}

	// checked in today: tonight's reminder goes, tomorrow's stays
	pastCheckin(t, s, sleep["id"].(int64), "2026-10-19", 0)
	rs, _ = s.Reminders(now, 3)
	if len(rs) != 4 || rs[0].ID != want[1] {
		t.Errorf("after check-in = %v", rs)
	}

	// capped below macOS's 64 pending notifications
	if rs, _ := s.Reminders(now, 100); len(rs) != MaxReminders {
		t.Errorf("100 days gave %d reminders, want %d", len(rs), MaxReminders)
	}
}

func TestDoses(t *testing.T) {
	s := open(t)
	med, err := s.Create("medications", Record{"name": "布洛芬缓释胶囊", "status": "按需"})
	if err != nil {
		t.Fatal(err)
	}
	mid := med["id"].(int64)
	d1, err := s.Create("doses", Record{"medication_id": mid, "taken_at": "2026-10-18 22:21"})
	if err != nil || d1["amount"] != 1.0 {
		t.Fatalf("dose without amount = %v %v, want 1", d1, err)
	}
	if _, err := s.Create("doses", Record{"medication_id": mid, "taken_at": "2026-10-19 08:05", "amount": 2}); err != nil {
		t.Fatal(err)
	}
	// an edit that leaves amount out keeps it
	if d, err := s.Update("doses", d1["id"].(int64), Record{"notes": "饭后"}); err != nil || d["amount"] != 1.0 {
		t.Errorf("edit = %v %v", d, err)
	}
	if _, err := s.Create("doses", Record{"taken_at": "now"}); err == nil {
		t.Error("dose without a medication accepted")
	}

	o, err := s.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if len(o.CurrentMeds) != 1 || o.CurrentMeds[0]["last_taken"] != "2026-10-19 08:05" || o.CurrentMeds[0]["taken_total"] != 3.0 {
		t.Errorf("current meds = %v", o.CurrentMeds)
	}
	rel, _ := s.Related("medications", mid)
	if ds, _ := rel["doses"].([]Record); len(ds) != 2 {
		t.Errorf("related doses = %v", rel["doses"])
	}
	tl, _ := s.Timeline(10)
	var titles []string
	for _, r := range tl {
		if r["type"] == "doses" {
			titles = append(titles, fmt.Sprint(r["title"]))
		}
	}
	if strings.Join(titles, "|") != "服用 布洛芬缓释胶囊 2 颗|服用 布洛芬缓释胶囊 1 颗" {
		t.Errorf("timeline doses = %v", titles)
	}

	// deleting the medication takes its doses with it
	if err := s.Delete("medications", mid); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := s.List("doses", ListQuery{}); n != 0 {
		t.Errorf("%d doses left after deleting the medication", n)
	}
}

func TestOverviewAndTimeline(t *testing.T) {
	s := open(t)
	c, _ := s.Create("conditions", Record{"name": "急性扁桃体炎", "status": "进行中", "onset_date": "2026-10-14"})
	s.Create("conditions", Record{"name": "阑尾炎", "status": "已痊愈"})
	s.Create("medications", Record{"name": "头孢", "status": "在用"})
	s.Create("medications", Record{"name": "多替诺雷", "status": "计划中"})
	s.Create("visits", Record{"visit_date": "2026-10-15", "hospital": "市一院", "diagnosis": "急性扁桃体炎", "condition_id": c["id"]})
	rep, _ := s.Create("reports", Record{"report_date": "2026-10-15", "title": "血常规"})
	s.Create("lab_results", Record{"report_id": rep["id"], "test_date": "2025-01-01", "item_name": "白细胞", "value_num": 15, "ref_range": "3.5-9.5"})
	s.Create("lab_results", Record{"report_id": rep["id"], "test_date": "2026-10-15", "item_name": "白细胞", "value_num": 6, "ref_range": "3.5-9.5"})
	s.Create("lab_results", Record{"report_id": rep["id"], "test_date": "2026-10-15", "item_name": "超敏C-反应蛋白", "value_text": "111.87↑", "ref_range": "0-6"})
	o, err := s.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if len(o.ActiveConditions) != 1 {
		t.Errorf("active conditions = %v", o.ActiveConditions)
	}
	// a planned drug isn't one being taken
	if len(o.CurrentMeds) != 1 || o.CurrentMeds[0]["name"] != "头孢" {
		t.Errorf("current meds = %v", o.CurrentMeds)
	}
	// only the latest result of an item counts: 白细胞 is back to normal
	if len(o.AbnormalLabs) != 1 || o.AbnormalLabs[0]["item_name"] != "超敏C-反应蛋白" {
		t.Errorf("abnormal labs = %v", o.AbnormalLabs)
	}
	trend, _ := s.LabTrend("反应蛋白")
	if len(trend) != 1 {
		t.Errorf("partial trend = %v", trend)
	}
	rel, _ := s.Related("conditions", c["id"].(int64))
	if len(rel["visits"].([]Record)) != 1 {
		t.Errorf("related = %v", rel)
	}
	tl, err := s.Timeline(10)
	if err != nil || len(tl) != 3 {
		t.Errorf("timeline = %v %v", tl, err)
	}
	hits, _ := s.Search("扁桃体", 10)
	if len(hits) != 2 {
		t.Errorf("search hits = %v", hits)
	}
}

// A check-in item can't be deleted, so its check-ins stay. Archiving takes
// it off the days (the 打卡 page), but the stats (全部打卡项, and MCP) still
// have yesterday's 喝水 after 喝水 is archived.
func TestHabitsAreArchivedNotDeleted(t *testing.T) {
	s := open(t)
	water, err := s.Create("habits", Record{"name": "喝水"})
	if err != nil {
		t.Fatal(err)
	}
	sleep, _ := s.Create("habits", Record{"name": "早睡"})
	wid, sid := water["id"].(int64), sleep["id"].(int64)
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	pastCheckin(t, s, wid, yesterday, 1)

	err = s.Delete("habits", wid)
	var ve *ValidationError
	if !errors.As(err, &ve) || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("Delete(habit) = %v, want a ValidationError pointing at archived", err)
	}
	if err := s.Delete("habits", 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete(missing habit) = %v, want ErrNotFound", err)
	}

	for _, id := range []int64{wid, sid} {
		if _, err := s.Update("habits", id, Record{"archived": true}); err != nil {
			t.Fatal(err)
		}
	}
	names := func(days []HabitDay) (out []string) {
		for _, d := range days {
			out = append(out, fmt.Sprint(d.Habit["name"]))
		}
		return out
	}
	for _, d := range []string{yesterday, today} {
		if days, err := s.HabitDays(d); err != nil || len(days) != 0 {
			t.Errorf("%s after archiving = %v %v, want none", d, names(days), err)
		}
	}
	stats, err := s.CheckinStats("", "", 0)
	if err != nil || len(stats) != 1 || stats[0].Habit["name"] != "喝水" || stats[0].DaysDone != 1 || stats[0].Dates[0] != yesterday {
		t.Errorf("30-day stats after archiving = %+v %v", stats, err)
	}
	if _, err := s.Checkin(sid, "today", nil, ""); err == nil || !strings.Contains(err.Error(), "已归档") {
		t.Errorf("checking in an archived item: err = %v", err)
	}
	if _, err := s.Create("habits", Record{"name": "喝水"}); err == nil || !strings.Contains(err.Error(), "恢复") {
		t.Errorf("a new item with an archived one's name: err = %v", err)
	}

	// restored, it's back today, and its history is whole
	if _, err := s.Update("habits", wid, Record{"archived": false}); err != nil {
		t.Fatal(err)
	}
	if days, _ := s.HabitDays(today); len(days) != 1 || days[0].Checkin != nil {
		t.Errorf("today after restoring = %v", names(days))
	}
	if _, n, _ := s.List("checkins", ListQuery{Filters: map[string]any{"habit_id": wid}}); n != 1 {
		t.Errorf("check-ins of 喝水 = %d, want 1", n)
	}
}

// BMI comes from each weight and the height of the time, and isn't recorded.
func TestBMI(t *testing.T) {
	s := open(t)
	s.Create("metrics", Record{"measured_at": "2026-10-01 08:00", "metric_type": "体重", "value": 100})
	if pts, err := s.MetricSeries(BMI, "", ""); err != nil || len(pts) != 0 {
		t.Fatalf("no height: series = %v %v, want none", pts, err)
	}
	if o, _ := s.Overview(); len(o.LatestMetrics) != 1 {
		t.Errorf("no height: latest = %v, want the weight only", o.LatestMetrics)
	}
	// a height measured later counts for the weights before it too
	s.Create("metrics", Record{"measured_at": "2026-10-10 08:00", "metric_type": "身高", "value": 179})
	s.Create("metrics", Record{"measured_at": "2026-10-20 08:00", "metric_type": "体重", "value": 186.6, "unit": "斤"})
	// a new height counts from then on; 1.80 in cm is taken as metres
	s.Create("metrics", Record{"measured_at": "2026-10-22 08:00", "metric_type": "身高", "value": 1.80})
	w, _ := s.Create("metrics", Record{"measured_at": "2026-10-25 09:32", "metric_type": "体重", "value": 93.3})
	// a unit it can't turn into kg is left out
	s.Create("metrics", Record{"measured_at": "2026-10-23 08:00", "metric_type": "体重", "value": 93000, "unit": "g"})

	pts, err := s.MetricSeries(BMI, "", "")
	if err != nil || len(pts) != 3 {
		t.Fatalf("series = %v %v, want 3", pts, err)
	}
	for i, want := range []struct {
		v        float64
		cat      string
		heightCM float64
	}{{31.2, "肥胖", 179}, {29.1, "肥胖", 179}, {28.8, "肥胖", 180}} {
		if p := pts[i]; p["value"] != want.v || p["category"] != want.cat || p["height_cm"] != want.heightCM || p["unit"] != "kg/m²" {
			t.Errorf("point %d = %v, want %v", i, p, want)
		}
	}
	if pts[2]["source"] != "metrics" || pts[2]["id"] != w["id"] {
		t.Errorf("last point = %v, want the weight #%v it is from", pts[2], w["id"])
	}
	if pts, _ := s.MetricSeries(BMI, "2026-10-21", ""); len(pts) != 1 || pts[0]["value"] != 28.8 {
		t.Errorf("series from 10-21 = %v", pts)
	}
	o, err := s.Overview()
	var latest Record
	for _, m := range o.LatestMetrics {
		if m["metric_type"] == BMI {
			latest = m
		}
	}
	if err != nil || latest == nil || latest["value"] != 28.8 || latest["measured_at"] != "2026-10-25 09:32" {
		t.Errorf("latest BMI = %v %v, want 28.8 of 10-25", latest, err)
	}

	for _, in := range []Record{
		{"measured_at": "today", "metric_type": "BMI", "value": 29},
		{"measured_at": "today", "metric_type": "bmi", "value": 29},
	} {
		if _, err := s.Create("metrics", in); err == nil || !strings.Contains(err.Error(), "自动算出") {
			t.Errorf("recording %v: err = %v, want told it's worked out", in["metric_type"], err)
		}
	}
	for v, want := range map[float64]string{18.4: "偏瘦", 18.5: "正常", 23.9: "正常", 24: "超重", 27.9: "超重", 28: "肥胖"} {
		if got := bmiCategory(v); got != want {
			t.Errorf("BMI %v is %s, want %s", v, got, want)
		}
	}
	for _, name := range []string{"BMI", " bmi ", "身高"} {
		if _, err := s.Create("metric_kinds", Record{"name": name}); err == nil {
			t.Errorf("metric kind %q was made", name)
		}
	}
}
