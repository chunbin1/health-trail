package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func outbox(t *testing.T, s *Store) map[string]string {
	t.Helper()
	rows, err := s.DB.Query("SELECT entity || '/' || uid, op FROM sync_outbox")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, op string
		if err := rows.Scan(&k, &op); err != nil {
			t.Fatal(err)
		}
		out[k] = op
	}
	return out
}

func uidOf(t *testing.T, s *Store, entity string, id int64) string {
	t.Helper()
	uid, err := s.UID(entity, id)
	if err != nil || !ValidUID(uid) {
		t.Fatalf("%s #%d: uid %q, %v", entity, id, uid, err)
	}
	return uid
}

func TestEveryRecordHasAUID(t *testing.T) {
	s := open(t)
	// a uid sent in is ignored, like an id
	c, err := s.Create("conditions", Record{"name": "胃炎", "uid": "0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c["uid"]; ok {
		t.Fatal("the uid is in the record")
	}
	uid := uidOf(t, s, "conditions", idOf(c))
	if uid == "0123456789abcdef0123456789abcdef" {
		t.Fatal("took the uid it was sent")
	}
	if id, err := s.IDByUID("conditions", uid); err != nil || id != idOf(c) {
		t.Fatalf("IDByUID: %d %v", id, err)
	}
	// a change keeps it
	if _, err := s.Update("conditions", idOf(c), Record{"status": "慢性", "uid": "x"}); err != nil {
		t.Fatal(err)
	}
	if got := uidOf(t, s, "conditions", idOf(c)); got != uid {
		t.Fatalf("uid changed: %s → %s", uid, got)
	}
	// rows that bypass Create get one too
	h, err := s.Create("habits", Record{"name": "晨跑"})
	if err != nil {
		t.Fatal(err)
	}
	ci, err := s.Checkin(idOf(h), "today", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	other := uidOf(t, s, "checkins", idOf(ci))
	if other == uid {
		t.Fatal("same uid twice")
	}
	if _, err := s.IDByUID("conditions", "not-a-uid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad uid: %v", err)
	}
	if _, err := s.IDByUID("conditions", other); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another table's uid: %v", err)
	}
}

// A database from before the uid: every row gets one when it's opened.
func TestOldDatabaseGetsUIDs(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE conditions (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, category TEXT, status TEXT,
			onset_date TEXT, resolved_date TEXT, notes TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`INSERT INTO conditions (name, created_at, updated_at) VALUES ('胃炎', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		`INSERT INTO conditions (name, created_at, updated_at) VALUES ('鼻炎', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, b := uidOf(t, s, "conditions", 1), uidOf(t, s, "conditions", 2)
	if a == b {
		t.Fatal("same uid")
	}
	// opening again changes nothing
	s.Close()
	if s, err = Open(dir); err != nil {
		t.Fatal(err)
	}
	if uidOf(t, s, "conditions", 1) != a {
		t.Fatal("uid changed on reopening")
	}
	if _, err := s.DB.Exec("UPDATE conditions SET uid = ? WHERE id = 2", a); err == nil {
		t.Fatal("two rows took the same uid")
	}
}

func TestOutboxNotesChangesOnlyWhenOn(t *testing.T) {
	s := open(t)
	med, err := s.Create("medications", Record{"name": "别嘌醇", "status": "在用"})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(outbox(t, s)); n != 0 {
		t.Fatalf("sync is off, outbox has %d", n)
	}
	if _, err := s.DB.Exec("INSERT INTO sync_state (key, value) VALUES ('enabled', '1')"); err != nil {
		t.Fatal(err)
	}
	cond, err := s.Create("conditions", Record{"name": "高尿酸血症"})
	if err != nil {
		t.Fatal(err)
	}
	sym, err := s.Create("symptoms", Record{"occurred_at": "2026-10-27 09:00", "description": "脚痛", "condition_id": idOf(cond)})
	if err != nil {
		t.Fatal(err)
	}
	dose, err := s.Create("doses", Record{"medication_id": idOf(med), "taken_at": "2026-10-27 09:00"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("medications", idOf(med), Record{"remind_times": "09:00"}); err != nil {
		t.Fatal(err)
	}
	medUID, condUID := uidOf(t, s, "medications", idOf(med)), uidOf(t, s, "conditions", idOf(cond))
	symUID, doseUID := uidOf(t, s, "symptoms", idOf(sym)), uidOf(t, s, "doses", idOf(dose))
	got := outbox(t, s)
	for k, want := range map[string]string{
		"medications/" + medUID: "save", "conditions/" + condUID: "save",
		"symptoms/" + symUID: "save", "doses/" + doseUID: "save",
	} {
		if got[k] != want {
			t.Errorf("%s: %q, want %q (%v)", k, got[k], want, got)
		}
	}
	if _, err := s.DB.Exec("DELETE FROM sync_outbox"); err != nil {
		t.Fatal(err)
	}
	// a cascade takes the doses; a reference set to null changes the symptom
	if err := s.Delete("medications", idOf(med)); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("conditions", idOf(cond)); err != nil {
		t.Fatal(err)
	}
	got = outbox(t, s)
	for k, want := range map[string]string{
		"medications/" + medUID: "delete", "doses/" + doseUID: "delete",
		"conditions/" + condUID: "delete", "symptoms/" + symUID: "save",
	} {
		if got[k] != want {
			t.Errorf("%s: %q, want %q (%v)", k, got[k], want, got)
		}
	}
	// a row written by something that knows nothing of uids (an older app)
	if _, err := s.DB.Exec(`INSERT INTO conditions (name, created_at, updated_at) VALUES ('鼻炎', '2026-10-28 00:00:00', '2026-10-28 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	var id int64
	s.DB.QueryRow("SELECT id FROM conditions WHERE name = '鼻炎'").Scan(&id)
	if op := outbox(t, s)["conditions/"+uidOf(t, s, "conditions", id)]; op != "save" {
		t.Fatalf("older app's row: %q", op)
	}
}

// Reading a record never shows the uid: MCP and the API have the id.
func TestRecordsHideTheUID(t *testing.T) {
	s := open(t)
	c, _ := s.Create("conditions", Record{"name": "胃炎"})
	r, err := s.Get("conditions", idOf(c))
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := s.List("conditions", ListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range append(rows, r) {
		if _, ok := rec["uid"]; ok {
			t.Fatalf("uid shown: %v", rec)
		}
	}
}
