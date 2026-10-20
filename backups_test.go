package main

import (
	"strings"
	"testing"
	"time"

	"github.com/lee/health-vault/internal/backup"
	"github.com/lee/health-vault/internal/store"
)

// With the default of keeping one backup, restoring must still work: the
// backup made of the current data first may not prune away the one being
// restored.
func TestRestoreKeepingOne(t *testing.T) {
	data := t.TempDir()
	st, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b := newBackups(st, data)
	s := b.load()
	if s.Config.Keep != 1 {
		t.Fatalf("default keep = %d, want 1", s.Config.Keep)
	}
	s.Config.Dir = t.TempDir()
	if err := b.save(s); err != nil {
		t.Fatal(err)
	}

	st.Create("conditions", store.Record{"name": "备份里的"})
	name, err := b.run("")
	if err != nil {
		t.Fatal(err)
	}
	st.Create("conditions", store.Record{"name": "备份之后加的"})
	time.Sleep(1100 * time.Millisecond) // a different second, so a different file name

	if err := b.restore(name); err != nil {
		t.Fatalf("restore with keep 1: %v", err)
	}
	if _, n, _ := st.List("conditions", store.ListQuery{}); n != 1 {
		t.Errorf("conditions after restore = %d, want 1", n)
	}
	es, _ := backup.List(s.Config.Dir)
	if len(es) != 2 || !strings.Contains(es[0].Name, backup.BeforeRestore) {
		t.Errorf("after restore = %+v, want the restored one and the before-restore one", es)
	}

	// the next backup trims the folder back to one
	time.Sleep(1100 * time.Millisecond)
	if _, err := b.run(""); err != nil {
		t.Fatal(err)
	}
	if es, _ := backup.List(s.Config.Dir); len(es) != 1 {
		t.Errorf("after the next backup = %+v, want 1", es)
	}
}
