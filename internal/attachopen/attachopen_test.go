package attachopen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lee/health-trail/internal/store"
)

func TestOpenAttachment(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rep, _ := st.Create("reports", store.Record{"report_date": "2026-10-22", "title": "MR"})
	add := func(name, mime, body string) int64 {
		a, err := st.AddAttachment("reports", rep["id"].(int64), name, mime, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return a["id"].(int64)
	}
	tmp := t.TempDir()
	base := Dir(tmp, "/data")
	var ran []string
	run := func(args ...string) error { ran = args; return nil }

	// a picture opens in its app, as a copy under its own name
	revealed, err := Open(st, add("报告单.JPG", "image/jpeg", "jpg bytes"), base, run)
	if err != nil || revealed || len(ran) != 1 {
		t.Fatalf("revealed=%v err=%v ran=%v", revealed, err, ran)
	}
	if filepath.Base(ran[0]) != "报告单.JPG" {
		t.Errorf("opened %s", ran[0])
	}
	if b, _ := os.ReadFile(ran[0]); string(b) != "jpg bytes" {
		t.Errorf("copy holds %q", b)
	}

	// a file that `open` might run is only shown in the Finder
	for _, name := range []string{"run.command", "noext", "page.html", ".jpg"} {
		revealed, err = Open(st, add(name, "image/png", "x"), base, run)
		if err != nil {
			t.Fatal(err)
		}
		if name == ".jpg" {
			if revealed || filepath.Base(ran[0]) != "attachment.jpg" {
				t.Errorf(".jpg: revealed=%v ran=%v", revealed, ran)
			}
			continue
		}
		if !revealed || len(ran) != 2 || ran[0] != "-R" {
			t.Errorf("%s: revealed=%v ran=%v", name, revealed, ran)
		}
	}

	if _, err := Open(st, 999, base, run); err != store.ErrNotFound {
		t.Errorf("missing attachment: %v", err)
	}
}

// The copies are health data in plain form: none may linger.
func TestOpenedCopiesGo(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rep, _ := st.Create("reports", store.Record{"report_date": "2026-10-22", "title": "MR"})
	add := func() int64 {
		a, _ := st.AddAttachment("reports", rep["id"].(int64), "报告.pdf", "application/pdf", strings.NewReader("%PDF"))
		return a["id"].(int64)
	}
	tmp := t.TempDir()
	base, other := Dir(tmp, "/data"), Dir(tmp, "/test-data")
	var ran []string
	run := func(args ...string) error { ran = args; return nil }
	open := func(id int64) string {
		if _, err := Open(st, id, base, run); err != nil {
			t.Fatal(err)
		}
		return filepath.Dir(ran[0])
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	a, b := add(), add()
	first, second := open(a), open(a)
	if first == second {
		t.Error("two opens share a folder")
	}
	other1 := filepath.Join(other, "1-x")
	os.MkdirAll(other1, 0o700)
	legacy := filepath.Join(tmp, "health-trail-open", "2") // an earlier version's
	os.MkdirAll(legacy, 0o700)

	// deleting an attachment removes its copies, not another's
	ofB := open(b)
	Forget(base, a)
	if exists(first) || exists(second) || !exists(ofB) {
		t.Errorf("after deleting %d: %v %v %v", a, exists(first), exists(second), exists(ofB))
	}
	// an old copy goes the next time something opens
	old := time.Now().Add(-2 * openedTTL)
	os.Chtimes(ofB, old, old)
	fresh := open(b)
	if exists(ofB) || !exists(fresh) {
		t.Errorf("sweep: old %v fresh %v", exists(ofB), exists(fresh))
	}
	// quitting removes the rest, and an earlier version's, but not another
	// data directory's (a test window's, say)
	Clear(base)
	if exists(base) || exists(legacy) || !exists(other1) {
		t.Errorf("clear: base %v legacy %v other %v", exists(base), exists(legacy), exists(other1))
	}
}
