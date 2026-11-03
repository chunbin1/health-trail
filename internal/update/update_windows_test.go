//go:build windows

package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnsignedExeDoesNotUpdate(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "health-trail.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := &Installer{App: exe}
	if _, err := in.Check(); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("Check(unsigned) = %v, want ErrUnsigned", err)
	}
}
