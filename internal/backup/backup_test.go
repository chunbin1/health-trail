package backup

import (
	"archive/zip"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lee/health-vault/internal/store"
)

func open(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func count(t *testing.T, s *store.Store, table string) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBackupAndRestore(t *testing.T) {
	s := open(t)
	c, _ := s.Create("conditions", store.Record{"name": "高尿酸血症"})
	cid := c["id"].(int64)
	s.Create("visits", store.Record{"visit_date": "2026-10-17", "condition_id": cid, "hospital": "南方医院"})
	med, _ := s.Create("medications", store.Record{"name": "布洛芬"})
	s.Create("doses", store.Record{"medication_id": med["id"], "taken_at": "2026-10-19 08:00"})
	att, err := s.AddAttachment("conditions", cid, "报告.txt", "text/plain", strings.NewReader("原件"))
	if err != nil {
		t.Fatal(err)
	}
	attPath := filepath.Join(s.FileDir, att["stored_name"].(string))

	dir := filepath.Join(t.TempDir(), "备份 文件夹") // a space, as in "Application Support"
	now := time.Date(2026, 10, 20, 23, 0, 5, 0, time.Local)
	name, err := Create(s, dir, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if name != "health-vault-2026-10-20-230005.zip" {
		t.Errorf("name = %q", name)
	}

	// afterwards: a record added, one deleted, the attachment gone
	s.Create("conditions", store.Record{"name": "备份之后才加的"})
	if err := s.Delete("medications", med["id"].(int64)); err != nil {
		t.Fatal(err)
	}
	os.Remove(attPath)

	if err := Restore(s, filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "conditions"); n != 1 {
		t.Errorf("conditions after restore = %d, want 1", n)
	}
	if n := count(t, s, "doses"); n != 1 {
		t.Errorf("doses after restore = %d, want 1 (deleted with the medication, then restored)", n)
	}
	v, _, _ := s.List("visits", store.ListQuery{})
	if v[0]["condition_id"] != cid || v[0]["hospital"] != "南方医院" {
		t.Errorf("visit after restore = %v", v[0])
	}
	if b, err := os.ReadFile(attPath); err != nil || string(b) != "原件" {
		t.Errorf("attachment after restore = %q %v", b, err)
	}
	// new records don't reuse a restored id
	c2, _ := s.Create("conditions", store.Record{"name": "恢复之后"})
	if c2["id"].(int64) <= cid {
		t.Errorf("new id %v reuses a restored one", c2["id"])
	}
}

func TestRestoreOlderBackup(t *testing.T) {
	s := open(t)
	s.Create("habits", store.Record{"name": "作息守住", "remind_at": "23:00"})
	dir := t.TempDir()
	name, err := Create(s, dir, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// make it look like a backup from before remind_at existed
	path := filepath.Join(dir, name)
	tmp := t.TempDir()
	unzip(t, path, tmp)
	db, _ := sql.Open("sqlite", filepath.Join(tmp, "health.db"))
	if _, err := db.Exec("ALTER TABLE habits DROP COLUMN remind_at"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	rezip(t, tmp, path)

	if err := Restore(s, path); err != nil {
		t.Fatal(err)
	}
	h, _, _ := s.List("habits", store.ListQuery{})
	if len(h) != 1 || h[0]["name"] != "作息守住" || h[0]["remind_at"] != nil {
		t.Errorf("habits = %v", h)
	}
}

func TestRestoreRefusesBadFiles(t *testing.T) {
	s := open(t)
	s.Create("conditions", store.Record{"name": "留着"})
	dir := t.TempDir()

	notOurs := filepath.Join(dir, "健康档案-x.zip")
	zipFiles(t, notOurs, map[string]string{"readme.txt": "hi"})
	if err := Restore(s, notOurs); err == nil || !strings.Contains(err.Error(), "health.db") {
		t.Errorf("zip without health.db: %v", err)
	}
	escape := filepath.Join(dir, "健康档案-y.zip")
	zipFiles(t, escape, map[string]string{"health.db": "", "files/../../evil": "x"})
	if err := Restore(s, escape); err == nil || !strings.Contains(err.Error(), "不安全") {
		t.Errorf("zip with ../ path: %v", err)
	}
	broken := filepath.Join(dir, "健康档案-z.zip")
	zipFiles(t, broken, map[string]string{"health.db": "not a database"})
	if err := Restore(s, broken); err == nil {
		t.Error("corrupt database restored")
	}
	if n := count(t, s, "conditions"); n != 1 {
		t.Errorf("a refused restore changed the data: %d conditions", n)
	}
}

func TestDirAndPrune(t *testing.T) {
	s := open(t)
	for _, d := range []string{"", "relative/dir", s.Dir, filepath.Join(s.Dir, "backups")} {
		if err := CheckDir(s, d); err == nil {
			t.Errorf("CheckDir(%q) accepted", d)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "别的文件.zip"), []byte("x"), 0o600)
	base := time.Date(2026, 10, 1, 21, 0, 0, 0, time.Local)
	for i := 0; i < 5; i++ {
		if _, err := Create(s, dir, "", base.AddDate(0, 0, i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := Prune(dir, 3); err != nil {
		t.Fatal(err)
	}
	es, _ := List(dir)
	if len(es) != 3 || es[0].Time != "2026-10-05 21:00:00" || es[2].Time != "2026-10-03 21:00:00" {
		t.Errorf("after prune = %+v", es)
	}
	if _, err := os.Stat(filepath.Join(dir, "别的文件.zip")); err != nil {
		t.Error("prune removed a file that isn't a backup")
	}

	// the first backups had Chinese names; they still list, by their time
	old := filepath.Join(dir, "健康档案-2026-09-30-210000-恢复前.zip")
	os.WriteFile(old, []byte("x"), 0o600)
	es, _ = List(dir)
	if len(es) != 4 || es[3].Name != filepath.Base(old) || es[3].Time != "2026-09-30 21:00:00" {
		t.Errorf("with an old backup = %+v", es)
	}
	Prune(dir, 3)
	if _, err := os.Stat(old); err == nil {
		t.Error("an old backup beyond the count was kept")
	}
}

func TestDue(t *testing.T) {
	loc := time.Local
	at := func(d, hm string) time.Time {
		t, _ := time.ParseInLocation("2006-01-02 15:04", d+" "+hm, loc)
		return t
	}
	daily := Config{Daily: true, DailyAt: "21:00"}
	weekly := Config{Weekly: true, Weekday: 0, WeeklyAt: "20:00"} // Sundays; 2026-10-18 is one
	cases := []struct {
		name      string
		c         Config
		last, now time.Time
		want      bool
	}{
		{"daily, before today's time, done yesterday", daily, at("2026-10-19", "21:00"), at("2026-10-20", "20:59"), false},
		{"daily, at today's time", daily, at("2026-10-19", "21:00"), at("2026-10-20", "21:00"), true},
		{"daily, already done after it", daily, at("2026-10-20", "21:30"), at("2026-10-20", "23:00"), false},
		{"daily, app closed for days", daily, at("2026-10-15", "21:00"), at("2026-10-20", "09:00"), true},
		{"daily, never backed up", daily, time.Time{}, at("2026-10-20", "09:00"), true},
		{"weekly, on a Monday after Sunday's", weekly, at("2026-10-11", "20:00"), at("2026-10-19", "08:00"), true},
		{"weekly, done Sunday", weekly, at("2026-10-18", "20:05"), at("2026-10-20", "08:00"), false},
		{"nothing on", Config{}, time.Time{}, at("2026-10-20", "09:00"), false},
	}
	for _, c := range cases {
		if got := Due(c.c, c.last, c.now); got != c.want {
			t.Errorf("%s: Due = %v, want %v", c.name, got, c.want)
		}
	}
	bad := Config{DailyAt: "25:00", WeeklyAt: "21:00", Keep: 30}
	if bad.Check() == nil {
		t.Error("25:00 accepted")
	}
	var empty Config
	empty.Defaults()
	if empty.Check() != nil || empty.Keep != 30 {
		t.Errorf("defaults = %+v", empty)
	}
}

func unzip(t *testing.T, path, to string) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if err := extract(f, filepath.Join(to, filepath.FromSlash(f.Name))); err != nil {
			t.Fatal(err)
		}
	}
}

func rezip(t *testing.T, from, path string) {
	t.Helper()
	files := map[string]string{}
	filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(from, p)
			b, _ := os.ReadFile(p)
			files[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	zipFiles(t, path, files)
}

func zipFiles(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range files {
		w, _ := z.Create(name)
		io.WriteString(w, body)
	}
	z.Close()
	f.Close()
}
