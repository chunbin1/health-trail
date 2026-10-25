package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	var ran []string
	run := func(args ...string) error { ran = args; return nil }

	// a picture opens in its app, as a copy under its own name
	revealed, err := openAttachment(st, add("报告单.JPG", "image/jpeg", "jpg bytes"), tmp, run)
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
		revealed, err = openAttachment(st, add(name, "image/png", "x"), tmp, run)
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

	if _, err := openAttachment(st, 999, tmp, run); err != store.ErrNotFound {
		t.Errorf("missing attachment: %v", err)
	}
}
