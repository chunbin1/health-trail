package store

import (
	"fmt"
	"strings"
	"time"
)

func today() string { return time.Now().Format("2006-01-02") }

// Overview is the state of things now: what is ongoing, what is recent and
// what is out of range. It is the dashboard, and Claude's first read.
type Overview struct {
	Today            string                    `json:"today"`
	ActiveConditions []Record                  `json:"active_conditions"`
	CurrentMeds      []Record                  `json:"current_medications"`
	RecentVisits     []Record                  `json:"recent_visits"`
	UpcomingFollowUp []Record                  `json:"upcoming_follow_ups"`
	Appointments     []Record                  `json:"upcoming_appointments"` // not done yet, from a week ago on
	MedsToday        []MedDay                  `json:"meds_today"`            // medicines with remind_times, and which times are taken
	RecentSymptoms   []Record                  `json:"recent_symptoms"`
	AbnormalLabs     []Record                  `json:"abnormal_labs_latest"`
	LatestMetrics    []Record                  `json:"latest_metrics"`
	MetricKinds      []Record                  `json:"metric_kinds"` // metrics the user added, with their units
	TodayCheckins    []HabitDay                `json:"today_checkins"`
	Counts           map[string]int            `json:"counts"`
}

type HabitDay struct {
	Habit   Record `json:"habit"`
	Checkin Record `json:"checkin"` // nil if not done
}

func (s *Store) Overview() (*Overview, error) {
	o := &Overview{Today: today(), Counts: map[string]int{}}
	var err error
	q := func(dst *[]Record, sqlText string, args ...any) {
		if err != nil {
			return
		}
		*dst, err = s.query(sqlText, args...)
	}
	q(&o.ActiveConditions, `SELECT * FROM conditions WHERE status IS NULL OR status != '已痊愈' ORDER BY onset_date DESC, id DESC`)
	// with when each was last taken and how many pills in all, from doses
	q(&o.CurrentMeds, `SELECT m.*,
		(SELECT MAX(taken_at) FROM doses WHERE medication_id = m.id) AS last_taken,
		(SELECT SUM(amount) FROM doses WHERE medication_id = m.id) AS taken_total
		FROM medications m WHERE (status IS NULL OR status NOT IN ('已停用', '计划中')) AND (end_date IS NULL OR end_date >= ?) ORDER BY start_date DESC`, today())
	q(&o.RecentVisits, `SELECT * FROM visits ORDER BY visit_date DESC, id DESC LIMIT 5`)
	q(&o.UpcomingFollowUp, `SELECT * FROM visits WHERE follow_up_date >= ? ORDER BY follow_up_date LIMIT 5`, today())
	q(&o.RecentSymptoms, `SELECT * FROM symptoms WHERE substr(occurred_at,1,10) >= ? ORDER BY occurred_at DESC LIMIT 10`,
		time.Now().AddDate(0, 0, -30).Format("2006-01-02"))
	// the most recent result of each item, kept when it is out of range
	q(&o.AbnormalLabs, `SELECT l.* FROM lab_results l
		JOIN (SELECT item_name, MAX(test_date || printf('%010d', id)) AS k FROM lab_results GROUP BY item_name) m
		  ON l.item_name = m.item_name AND l.test_date || printf('%010d', l.id) = m.k
		WHERE l.flag IN ('偏高','偏低','异常') ORDER BY l.test_date DESC, l.item_name`)
	q(&o.LatestMetrics, `SELECT x.* FROM metrics x
		JOIN (SELECT metric_type, MAX(measured_at || printf('%010d', id)) AS k FROM metrics GROUP BY metric_type) m
		  ON x.metric_type = m.metric_type AND x.measured_at || printf('%010d', x.id) = m.k
		ORDER BY x.metric_type`)
	q(&o.MetricKinds, `SELECT * FROM metric_kinds ORDER BY id`)
	if err != nil {
		return nil, err
	}
	o.TodayCheckins, err = s.HabitDays(today())
	if err != nil {
		return nil, err
	}
	if o.Appointments, err = s.UpcomingAppointments(time.Now()); err != nil {
		return nil, err
	}
	if o.MedsToday, err = s.MedsToday(time.Now()); err != nil {
		return nil, err
	}
	for _, e := range Entities {
		var n int
		s.DB.QueryRow("SELECT COUNT(*) FROM " + e.Name).Scan(&n)
		o.Counts[e.Name] = n
	}
	return o, nil
}

// HabitDays lists the active habits with that day's check-in, if any, then
// the archived ones checked in that day: archiving stops a habit, it doesn't
// take back the days it was done.
func (s *Store) HabitDays(date string) ([]HabitDay, error) {
	habits, err := s.query(`SELECT * FROM habits WHERE archived IS NULL OR archived = 0
		OR id IN (SELECT habit_id FROM checkins WHERE date = ?) ORDER BY COALESCE(archived, 0), id`, date)
	if err != nil {
		return nil, err
	}
	out := []HabitDay{}
	for _, h := range habits {
		cs, err := s.query(`SELECT * FROM checkins WHERE habit_id = ? AND date = ?`, h["id"], date)
		if err != nil {
			return nil, err
		}
		hd := HabitDay{Habit: h}
		if len(cs) > 0 {
			hd.Checkin = cs[0]
		}
		out = append(out, hd)
	}
	return out, nil
}

// FindHabit finds a habit by id or by name (exact, then partial match).
func (s *Store) FindHabit(ref string) (Record, error) {
	ref = strings.TrimSpace(ref)
	var id int64
	if _, err := fmt.Sscanf(ref, "%d", &id); err == nil && fmt.Sprint(id) == ref {
		return s.Get("habits", id)
	}
	rows, err := s.query(`SELECT * FROM habits WHERE name = ?`, ref)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		rows, err = s.query(`SELECT * FROM habits WHERE name LIKE ? ORDER BY archived, id`, "%"+ref+"%")
		if err != nil {
			return nil, err
		}
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows[0], nil
}

// Checkin records a habit as done on date, or updates that day's value and
// note when it already is. For a habit that logs a metric (metric_type), a
// value is also kept as that day's metric record, linked by metric_id.
func (s *Store) Checkin(habitID int64, date string, value *float64, notes string) (Record, error) {
	d, err := normalizeDate(date)
	if err != nil {
		return nil, invalid("%v", err)
	}
	// before the metric below is touched
	ce, _ := entity("checkins")
	if err := editable(ce, Record{"date": d}); err != nil {
		return nil, err
	}
	h, err := s.Get("habits", habitID)
	if err != nil {
		return nil, err
	}
	if archived(h) {
		return nil, invalid("「%v」已归档，不能打卡；要继续打卡，先把它的 archived 改成 false（恢复）", h["name"])
	}
	rows, err := s.query(`SELECT * FROM checkins WHERE habit_id = ? AND date = ?`, habitID, d)
	if err != nil {
		return nil, err
	}
	in := Record{"habit_id": habitID, "date": d}
	if value != nil {
		in["value"] = *value
	}
	if notes != "" {
		in["notes"] = notes
	}
	var newMetric int64
	if mt, ok := h["metric_type"].(string); ok && value != nil {
		var mid int64
		if len(rows) > 0 {
			mid, _ = rows[0]["metric_id"].(int64)
		}
		if mid > 0 && s.mustExist("metrics", mid) == nil {
			if _, err := s.Update("metrics", mid, Record{"value": *value}); err != nil {
				return nil, err
			}
		} else {
			at := d
			if d == today() {
				at = time.Now().Format("2006-01-02 15:04")
			}
			m, err := s.Create("metrics", Record{"measured_at": at, "metric_type": mt, "value": *value, "context": h["name"]})
			if err != nil {
				return nil, err
			}
			newMetric = m["id"].(int64)
			in["metric_id"] = newMetric
		}
	}
	var c Record
	if len(rows) > 0 {
		c, err = s.Update("checkins", rows[0]["id"].(int64), in)
	} else {
		c, err = s.Create("checkins", in)
	}
	if err != nil && newMetric > 0 {
		s.Delete("metrics", newMetric)
	}
	return c, err
}

// Uncheck removes a habit's check-in on date, and the metric record it
// logged, if any. It reports whether there was one.
func (s *Store) Uncheck(habitID int64, date string) (bool, error) {
	d, err := normalizeDate(date)
	if err != nil {
		return false, invalid("%v", err)
	}
	rows, err := s.query(`SELECT * FROM checkins WHERE habit_id = ? AND date = ?`, habitID, d)
	if err != nil || len(rows) == 0 {
		return false, err
	}
	if err := s.Delete("checkins", rows[0]["id"].(int64)); err != nil {
		return false, err
	}
	if mid, ok := rows[0]["metric_id"].(int64); ok {
		if err := s.Delete("metrics", mid); err != nil && err != ErrNotFound {
			return true, err
		}
	}
	return true, nil
}

type HabitStats struct {
	Habit         Record   `json:"habit"`
	DaysDone      int      `json:"days_done"`
	DaysInRange   int      `json:"days_in_range"`
	Rate          float64  `json:"completion_rate"`
	CurrentStreak int      `json:"current_streak"`
	LongestStreak int      `json:"longest_streak_in_range"`
	Total         *float64 `json:"total_value,omitempty"`
	Dates         []string `json:"dates"`
}

// CheckinStats summarises each active habit, and each archived one checked
// in within the range (or the one given), over [from, to]. The current streak counts back from today, or from yesterday
// when today isn't done yet: an unfinished today doesn't break it.
func (s *Store) CheckinStats(from, to string, habitID int64) ([]HabitStats, error) {
	var err error
	if to == "" {
		to = today()
	} else if to, err = normalizeDate(to); err != nil {
		return nil, invalid("%v", err)
	}
	if from == "" {
		t, _ := time.ParseInLocation("2006-01-02", to, time.Local)
		from = t.AddDate(0, 0, -29).Format("2006-01-02")
	} else if from, err = normalizeDate(from); err != nil {
		return nil, invalid("%v", err)
	}
	var habits []Record
	if habitID > 0 {
		h, err := s.Get("habits", habitID)
		if err != nil {
			return nil, err
		}
		habits = []Record{h}
	} else if habits, err = s.query(`SELECT * FROM habits WHERE archived IS NULL OR archived = 0
		OR id IN (SELECT habit_id FROM checkins WHERE date BETWEEN ? AND ?) ORDER BY COALESCE(archived, 0), id`, from, to); err != nil {
		return nil, err
	}
	f, _ := time.ParseInLocation("2006-01-02", from, time.Local)
	t, _ := time.ParseInLocation("2006-01-02", to, time.Local)
	days := int(t.Sub(f).Hours()/24) + 1
	out := []HabitStats{}
	for _, h := range habits {
		st := HabitStats{Habit: h, DaysInRange: days, Dates: []string{}}
		rows, err := s.query(`SELECT date, value FROM checkins WHERE habit_id = ? AND date BETWEEN ? AND ? ORDER BY date`, h["id"], from, to)
		if err != nil {
			return nil, err
		}
		var total float64
		hasTotal := false
		run, prev := 0, ""
		for _, r := range rows {
			d := r["date"].(string)
			st.Dates = append(st.Dates, d)
			if v, ok := r["value"].(float64); ok {
				total += v
				hasTotal = true
			}
			if prev != "" && nextDay(prev) == d {
				run++
			} else {
				run = 1
			}
			if run > st.LongestStreak {
				st.LongestStreak = run
			}
			prev = d
		}
		st.DaysDone = len(rows)
		if days > 0 {
			st.Rate = float64(st.DaysDone) / float64(days)
		}
		// readings such as 体重 don't add up
		if hasTotal && h["metric_type"] == nil {
			st.Total = &total
		}
		st.CurrentStreak, err = s.streak(h["id"].(int64))
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

// archived says whether a habit is archived (a Bool field reads back as 0/1).
func archived(h Record) bool {
	switch v := h["archived"].(type) {
	case int64:
		return v != 0
	case bool:
		return v
	}
	return false
}

func nextDay(d string) string {
	t, _ := time.ParseInLocation("2006-01-02", d, time.Local)
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

func (s *Store) streak(habitID int64) (int, error) {
	rows, err := s.query(`SELECT date FROM checkins WHERE habit_id = ? AND date <= ? ORDER BY date DESC LIMIT 1000`, habitID, today())
	if err != nil {
		return 0, err
	}
	day := time.Now()
	n := 0
	for i, r := range rows {
		d := r["date"].(string)
		if i == 0 && d != day.Format("2006-01-02") {
			day = day.AddDate(0, 0, -1) // today not done yet
		}
		if d != day.Format("2006-01-02") {
			break
		}
		n++
		day = day.AddDate(0, 0, -1)
	}
	return n, nil
}

type LabItem struct {
	Name     string   `json:"item_name"`
	Count    int      `json:"count"`
	Latest   string   `json:"latest_date"`
	Value    *float64 `json:"latest_value"`
	Unit     *string  `json:"unit"`
	Flag     *string  `json:"latest_flag"`
	Panel    *string  `json:"panel"`
}

// LabItems lists every lab item ever measured with its latest result:
// the ones out of range now first, then the most measured.
func (s *Store) LabItems() ([]LabItem, error) {
	rows, err := s.DB.Query(`SELECT l.item_name, m.n, l.test_date, l.value_num, l.unit, l.flag, l.panel FROM lab_results l
		JOIN (SELECT item_name, COUNT(*) n, MAX(test_date || printf('%010d', id)) AS k FROM lab_results GROUP BY item_name) m
		  ON l.item_name = m.item_name AND l.test_date || printf('%010d', l.id) = m.k
		ORDER BY l.flag IN ('偏高','偏低','异常') DESC, m.n DESC, l.panel, l.item_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LabItem{}
	for rows.Next() {
		var it LabItem
		if err := rows.Scan(&it.Name, &it.Count, &it.Latest, &it.Value, &it.Unit, &it.Flag, &it.Panel); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// LabTrend returns every result of an item, oldest first. The name matches
// exactly when such an item exists, else as a substring, so "CRP" or
// "白细胞" find their item without the full printed name.
func (s *Store) LabTrend(name string) ([]Record, error) {
	rows, err := s.query(`SELECT l.*, r.title AS report_title FROM lab_results l LEFT JOIN reports r ON r.id = l.report_id
		WHERE l.item_name = ? ORDER BY l.test_date, l.id`, name)
	if err != nil || len(rows) > 0 {
		return rows, err
	}
	return s.query(`SELECT l.*, r.title AS report_title FROM lab_results l LEFT JOIN reports r ON r.id = l.report_id
		WHERE l.item_name LIKE ? ORDER BY l.item_name, l.test_date, l.id`, "%"+name+"%")
}

// MetricSeries returns one metric's readings in [from, to], oldest first.
func (s *Store) MetricSeries(metricType, from, to string) ([]Record, error) {
	rows, _, err := s.List("metrics", ListQuery{
		Filters: map[string]any{"metric_type": metricType}, From: from, To: to, Asc: true, Limit: 1000,
	})
	return rows, err
}

type SearchHit struct {
	Type  string `json:"type"`
	Label string `json:"type_label"`
	Rows  []Record `json:"rows"`
	Total int    `json:"total"`
}

// Search looks for text in every kind of record.
func (s *Store) Search(q string, limit int) ([]SearchHit, error) {
	if strings.TrimSpace(q) == "" {
		return nil, invalid("搜索词不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	out := []SearchHit{}
	for _, e := range Entities {
		if e.Name == "checkins" {
			continue
		}
		rows, total, err := s.List(e.Name, ListQuery{Q: q, Limit: limit})
		if err != nil {
			return nil, err
		}
		if total > 0 {
			out = append(out, SearchHit{Type: e.Name, Label: e.Label, Rows: rows, Total: total})
		}
	}
	return out, nil
}

// Related returns the record with what hangs off it: a condition's
// symptoms, visits and medications; a visit's medications and reports; a
// report's lab results. Attachments come with every record.
func (s *Store) Related(entityName string, id int64) (Record, error) {
	r, err := s.Get(entityName, id)
	if err != nil {
		return nil, err
	}
	out := Record{"record": r}
	children := map[string][][2]string{
		"conditions": {{"symptoms", "condition_id"}, {"visits", "condition_id"}, {"medications", "condition_id"}, {"appointments", "condition_id"}},
		"visits":     {{"medications", "visit_id"}, {"reports", "visit_id"}},
		"reports":    {{"lab_results", "report_id"}},
		"habits":     {{"checkins", "habit_id"}},
		"medications": {{"doses", "medication_id"}},
	}
	for _, c := range children[entityName] {
		rows, _, err := s.List(c[0], ListQuery{Filters: map[string]any{c[1]: id}, Limit: 1000, Asc: c[0] == "lab_results"})
		if err != nil {
			return nil, err
		}
		out[c[0]] = rows
	}
	att, err := s.Attachments(entityName, id)
	if err != nil {
		return nil, err
	}
	out["attachments"] = att
	return out, nil
}

// Timeline lists recent records of every kind, newest first, as one stream:
// what happened, so not appointments (ahead, or done as a visit or report).
func (s *Store) Timeline(limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 30
	}
	var parts []string
	for _, e := range Entities {
		if e.DateKey == "" || e.Name == "checkins" || e.Name == "lab_results" || e.Name == "appointments" {
			continue
		}
		title := e.TitleKey
		switch e.Name {
		case "visits":
			title = "COALESCE(diagnosis, department, hospital, visit_type, '就诊')"
		case "metrics":
			title = "metric_type || ' ' || value || COALESCE('/' || value2, '') || COALESCE(' ' || unit, '')"
		case "doses":
			title = "'服用 ' || COALESCE((SELECT name FROM medications WHERE id = medication_id), '药') || ' ' || printf('%g', amount) || ' 颗'"
		}
		parts = append(parts, fmt.Sprintf(`SELECT '%s' AS type, id, %s AS date, %s AS title FROM %s WHERE %s IS NOT NULL`,
			e.Name, e.DateKey, title, e.Name, e.DateKey))
	}
	return s.query(strings.Join(parts, " UNION ALL ")+" ORDER BY date DESC, id DESC LIMIT ?", limit)
}
