//go:build windows

package update

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The relaunch for real: a process that calls Relaunch and quits at once,
// as the app does, and the new health-trail.exe starting after it with the
// arguments, the old copy's folder gone. The test binary plays all three:
// the app (RELAUNCH_ROLE=app), the new exe (run as health-trail.exe) and
// the test.
func TestRelaunchStartsTheNewExe(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Health Trail")
	stage := filepath.Join(dir, stagePrefix+"1")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stage, "old.exe"), []byte("MZ"), 0o644)
	exe := filepath.Join(dir, windowsExe)
	copyFile(t, os.Args[0], exe)
	marker := filepath.Join(dir, "started.txt")

	app := exec.Command(os.Args[0], "-test.run=^TestRelaunchHelper$")
	app.Env = append(os.Environ(), "RELAUNCH_ROLE=app", "RELAUNCH_EXE="+exe, "RELAUNCH_STAGE="+stage, "RELAUNCH_MARKER="+marker)
	if out, err := app.CombinedOutput(); err != nil {
		t.Fatalf("the app: %v\n%s", err, out)
	}
	var got []byte
	for i := 0; i < 150 && got == nil; i++ {
		got, _ = os.ReadFile(marker)
		time.Sleep(200 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("the new exe didn't start within 30s")
	}
	if want := `-data|C:\Health Data|it's “quoted”`; string(got) != want {
		t.Errorf("the new exe got %q, want %q", got, want)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Errorf("the old copy's folder is still there: %v", err)
	}
}

func TestRelaunchHelper(t *testing.T) {
	if strings.EqualFold(filepath.Base(os.Args[0]), windowsExe) {
		// the new exe: what it was started with, after the test flags
		args := os.Args[1:]
		for i, a := range args {
			if a == "--" {
				args = args[i+1:]
				break
			}
		}
		os.WriteFile(os.Getenv("RELAUNCH_MARKER"), []byte(strings.Join(args, "|")), 0o644)
		return
	}
	if os.Getenv("RELAUNCH_ROLE") != "app" {
		t.Skip("run by TestRelaunchStartsTheNewExe")
	}
	args := []string{"-test.run=^TestRelaunchHelper$", "--", "-data", `C:\Health Data`, "it's “quoted”"}
	if err := Relaunch(os.Getenv("RELAUNCH_EXE"), os.Getenv("RELAUNCH_STAGE"), args); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
}
