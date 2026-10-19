package store

import (
	"errors"
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

// The CBC + CRP report the app was designed from, as a phone shows it.
var sampleRows = [][4]string{
	{"超敏C-反应蛋白", "111.87↑", "mg/L", "0-6"},
	{"白细胞", "17.62↑", "10^9/L", "3.5-9.5"},
	{"嗜中性粒细胞绝对值", "13.86↑", "10^9/L", "1.8-6.3"},
	{"淋巴细胞绝对值", "2.48", "10^9/L", "1.1-3.2"},
	{"单核细胞绝对值", "1.22↑", "10^9/L", "0.1-0.6"},
	{"嗜酸性粒细胞绝对值", "0.04", "10^9/L", "0.02-0.52"},
	{"嗜碱性粒细胞绝对值", "0.02", "10^9/L", "0-0.06"},
	{"嗜中性粒细胞比例", "78.7↑", "%", "40.0-75.0"},
	{"淋巴细胞比例", "14.1↓", "%", "20-50"},
	{"单核细胞比例", "6.9", "%", "3.0-10.0"},
	{"嗜酸性粒细胞比例", "0.2↓", "%", "0.4-8.0"},
	{"嗜碱性粒细胞比例", "0.1", "%", "0-1.0"},
	{"红细胞", "5.15", "10^12/L", "4.3-5.8"},
	{"血红蛋白", "153", "g/L", "130-175"},
	{"红细胞压积", "44.1", "%", "40-50"},
	{"红细胞平均体积", "85.7", "fl", "82-100"},
	{"红细胞平均血红蛋白", "29.8", "pg", "27-34"},
	{"平均血红蛋白浓度", "347", "g/L", "316-354"},
	{"红细胞分布宽度标准数", "37.5↓", "fl", "39-53.9"},
	{"红细胞分布宽度变异数", "12.2", "%", "11.0-18.0"},
	{"血小板", "242", "10^9/L", "125-350"},
	{"血小板平均体积", "9.8", "fl", "6.0-13"},
	{"血小板容积比", "0.237", "%", "0.16-0.4"},
	{"血小板分布宽度", "16.6", "GSD", "5-18"},
	{"大型血小板比率", "25.2", "%", "19.1-47"},
}

func checkSample(t *testing.T, rows []LabRow) {
	t.Helper()
	if len(rows) != len(sampleRows) {
		for _, r := range rows {
			t.Logf("%+v", r)
		}
		t.Fatalf("got %d rows, want %d", len(rows), len(sampleRows))
	}
	for i, want := range sampleRows {
		r := rows[i]
		n, flag, _ := ParseValue(want[1])
		if r.Name != want[0] || r.Value == nil || *r.Value != *n || r.Flag != flag || r.Unit != want[2] || r.RefRange != want[3] {
			t.Errorf("row %d = %+v (value %v), want %v", i, r, deref(r.Value), want)
		}
	}
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func TestParseLabTextTable(t *testing.T) {
	var b strings.Builder
	b.WriteString("检查项目\t结果\t单位\t参考值\n")
	for _, r := range sampleRows {
		b.WriteString(strings.Join(r[:], "\t") + "\n")
	}
	checkSample(t, ParseLabText(b.String()))
}

// Copied from a phone: one cell per line, long names broken in two, the
// arrow on a line of its own, and a range with spaces around its dash.
func TestParseLabTextCellPerLine(t *testing.T) {
	text := `检查项目
结果
单位
参考值
超敏C-反应蛋白
111.87
↑
mg/L
0-6
白细胞
17.62 ↑
10^9/L
3.5 - 9.5
嗜中性粒细胞绝
对值
13.86
↑
10^9/L
1.8-6.3
`
	rows := ParseLabText(text)
	checkSample(t, append(rows, ParseLabText(rest(3))...))
}

func rest(from int) string {
	var b strings.Builder
	for _, r := range sampleRows[from:] {
		name := []rune(r[0])
		if len(name) > 6 { // wrap like the phone did
			b.WriteString(string(name[:6]) + "\n" + string(name[6:]) + "\n")
		} else {
			b.WriteString(r[0] + "\n")
		}
		b.WriteString(r[1] + "\n" + r[2] + "\n" + r[3] + "\n")
	}
	return b.String()
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

func TestCheckinsAndStreak(t *testing.T) {
	s := open(t)
	h, err := s.Create("habits", Record{"name": "跑步", "unit": "公里"})
	if err != nil {
		t.Fatal(err)
	}
	id := h["id"].(int64)
	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }
	five := 5.0
	for _, n := range []int{-1, -2, -3, -5} {
		if _, err := s.Checkin(id, day(n), &five, ""); err != nil {
			t.Fatal(err)
		}
	}
	// checking in twice on a day updates it
	ten := 10.0
	if _, err := s.Checkin(id, day(-1), &ten, "加量"); err != nil {
		t.Fatal(err)
	}
	st, err := s.CheckinStats(day(-6), day(0), id)
	if err != nil {
		t.Fatal(err)
	}
	// today isn't done yet, so the streak runs back from yesterday
	if st[0].DaysDone != 4 || st[0].CurrentStreak != 3 || st[0].LongestStreak != 3 || *st[0].Total != 25 {
		t.Errorf("stats = %+v", st[0])
	}
	if _, err := s.Checkin(id, day(0), nil, ""); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.CheckinStats("", "", id); st[0].CurrentStreak != 4 {
		t.Errorf("streak with today = %d, want 4", st[0].CurrentStreak)
	}
	if ok, _ := s.Uncheck(id, day(0)); !ok {
		t.Error("uncheck found nothing")
	}
	if _, err := s.FindHabit("跑"); err != nil {
		t.Errorf("partial habit name: %v", err)
	}
}

// A habit with a metric_type logs its value as a metric record too: one per
// day, updated on a second check-in, removed with the check-in.
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
	// no value: a plain check-in, no metric
	if c, err = s.Checkin(id, "2026-01-02", nil, ""); err != nil || c["metric_id"] != nil {
		t.Errorf("check-in without value = %v %v", c, err)
	}
	if ok, err := s.Uncheck(id, "today"); !ok || err != nil {
		t.Fatalf("uncheck = %v %v", ok, err)
	}
	if _, n, _ := s.List("metrics", ListQuery{}); n != 0 {
		t.Errorf("metric left after uncheck: %d", n)
	}
	if _, err := s.Create("habits", Record{"name": "量血压", "metric_type": "血压"}); err == nil {
		t.Error("血压 needs two values and should not be a habit metric")
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
