package backup

import (
	"fmt"
	"time"
)

// Config is where backups go and what makes one. Every trigger can be on or
// off; 立即备份 in the app always works.
type Config struct {
	Dir      string `json:"dir"`
	Keep     int    `json:"keep"` // the newest this many are kept
	Daily    bool   `json:"daily"`
	DailyAt  string `json:"daily_at"` // HH:MM
	Weekly   bool   `json:"weekly"`
	Weekday  int    `json:"weekday"` // 0 Sunday … 6 Saturday
	WeeklyAt string `json:"weekly_at"`
	OnChange bool   `json:"on_change"` // once the data has been still for Quiet
	OnQuit   bool   `json:"on_quit"`
}

// Quiet is how long the data must go unchanged before an on-change backup,
// so a burst of edits makes one backup.
const Quiet = 10 * time.Minute

// Defaults fills in what an empty config leaves out.
func (c *Config) Defaults() {
	if c.Keep <= 0 {
		c.Keep = 30
	}
	if c.DailyAt == "" {
		c.DailyAt = "21:00"
	}
	if c.WeeklyAt == "" {
		c.WeeklyAt = "21:00"
	}
}

// Check refuses a config with an impossible time, weekday or count.
func (c *Config) Check() error {
	for _, at := range []string{c.DailyAt, c.WeeklyAt} {
		if _, err := time.Parse("15:04", at); err != nil {
			return fmt.Errorf("时间要写成 HH:MM，如 21:00，收到 %q", at)
		}
	}
	if c.Weekday < 0 || c.Weekday > 6 {
		return fmt.Errorf("星期要在 0（周日）到 6（周六）之间，收到 %d", c.Weekday)
	}
	if c.Keep < 1 || c.Keep > 1000 {
		return fmt.Errorf("保留份数要在 1 到 1000 之间，收到 %d", c.Keep)
	}
	return nil
}

// Due says whether a scheduled backup is owed at now: the latest daily or
// weekly time that has passed came after the last backup (of any kind). So a
// time missed while the app was closed is made up when it opens, and a backup
// made by hand after that time counts.
func Due(c Config, last, now time.Time) bool {
	if c.Daily {
		if slot, ok := latest(now, c.DailyAt, -1); ok && last.Before(slot) {
			return true
		}
	}
	if c.Weekly {
		if slot, ok := latest(now, c.WeeklyAt, c.Weekday); ok && last.Before(slot) {
			return true
		}
	}
	return false
}

// latest is the last moment at or before now that falls at HH:MM, on the
// given weekday (or any day, for -1).
func latest(now time.Time, at string, weekday int) (time.Time, bool) {
	hm, err := time.Parse("15:04", at)
	if err != nil {
		return time.Time{}, false
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, now.Location())
	for t.After(now) || (weekday >= 0 && int(t.Weekday()) != weekday) {
		t = t.AddDate(0, 0, -1)
	}
	return t, true
}
