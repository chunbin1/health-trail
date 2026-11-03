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

// TestMain lets the test binary stand in for the new health-trail.exe: run
// under that name, it writes down its arguments and quits.
func TestMain(m *testing.M) {
	if strings.EqualFold(filepath.Base(os.Args[0]), windowsExe) {
		os.WriteFile(os.Getenv("RELAUNCH_MARKER"), []byte(strings.Join(os.Args[1:], "|")), 0o644)
		os.Exit(0)
	}
	if os.Getenv("RELAUNCH_ROLE") == "app" {
		// the app: start the helper and quit at once
		var args []string
		if a := os.Getenv("RELAUNCH_ARGS"); a != "" {
			args = strings.Split(a, "|")
		}
		if err := Relaunch(os.Getenv("RELAUNCH_EXE"), os.Getenv("RELAUNCH_STAGE"), args); err != nil {
			os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type relaunchCase struct {
	dir, exe, stage, marker string
}

func newRelaunchCase(t *testing.T) relaunchCase {
	dir := filepath.Join(t.TempDir(), "Health Trail")
	c := relaunchCase{dir: dir, exe: filepath.Join(dir, windowsExe), stage: filepath.Join(dir, stagePrefix+"1"), marker: filepath.Join(dir, "started.txt")}
	if err := os.MkdirAll(c.stage, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(c.stage, "old.exe"), []byte("MZ"), 0o644)
	copyFile(t, os.Args[0], c.exe)
	t.Setenv("RELAUNCH_MARKER", c.marker)
	return c
}

// started waits for the new exe and says what it was started with.
func (c relaunchCase) started(t *testing.T) string {
	t.Helper()
	for i := 0; i < 100; i++ {
		if got, err := os.ReadFile(c.marker); err == nil {
			if _, err := os.Stat(c.stage); !os.IsNotExist(err) {
				t.Errorf("the old copy's folder is still there: %v", err)
			}
			return string(got)
		}
		time.Sleep(200 * time.Millisecond)
	}
	_, err := os.Stat(c.stage)
	t.Fatalf("the new exe didn't start within 20s (old copy's folder gone: %v)", os.IsNotExist(err))
	return ""
}

var relaunchArgCases = []struct {
	name string
	args []string
	want string
}{
	{"no arguments", nil, ""},
	{"arguments", []string{"-data", `C:\Health Data`, "it's “quoted”"}, `-data|C:\Health Data|it's “quoted”`},
}

// The script itself, run and waited for, after the app has gone.
func TestRelaunchScriptRuns(t *testing.T) {
	gone := exec.Command("cmd", "/c", "exit")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	for _, a := range relaunchArgCases {
		t.Run(a.name, func(t *testing.T) {
			c := newRelaunchCase(t)
			script := relaunchScript(gone.Process.Pid, c.exe, c.stage, a.args)
			out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
			if err != nil || len(out) > 0 {
				t.Logf("powershell: %v\n%s", err, out)
			}
			if got := c.started(t); got != a.want {
				t.Errorf("the new exe got %q, want %q", got, a.want)
			}
		})
	}
}

// The relaunch for real: a process that calls Relaunch and quits at once,
// as the app does, and the new health-trail.exe starting after it.
func TestRelaunchStartsTheNewExe(t *testing.T) {
	for _, a := range relaunchArgCases {
		t.Run(a.name, func(t *testing.T) {
			c := newRelaunchCase(t)
			app := exec.Command(os.Args[0])
			app.Env = append(os.Environ(), "RELAUNCH_ROLE=app", "RELAUNCH_EXE="+c.exe, "RELAUNCH_STAGE="+c.stage, "RELAUNCH_ARGS="+strings.Join(a.args, "|"))
			if out, err := app.CombinedOutput(); err != nil {
				t.Fatalf("the app: %v\n%s", err, out)
			}
			if got := c.started(t); got != a.want {
				t.Errorf("the new exe got %q, want %q", got, a.want)
			}
		})
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
