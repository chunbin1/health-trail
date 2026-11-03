//go:build windows

package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// testKeys are a Run key and a StartupApproved key of the test's own under
// HKCU, removed afterwards.
func testKeys(t *testing.T) (run, approved string) {
	t.Helper()
	base := `Software\HealthTrailTest\` + t.Name()
	run, approved = base+`\Run`, base+`\StartupApproved`
	for _, p := range []string{run, approved} {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, p, registry.ALL_ACCESS)
		if err != nil {
			t.Fatal(err)
		}
		k.Close()
	}
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, run)
		registry.DeleteKey(registry.CURRENT_USER, approved)
		registry.DeleteKey(registry.CURRENT_USER, base)
	})
	return run, approved
}

func setValue(t *testing.T, path, name string, v any) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	switch v := v.(type) {
	case string:
		err = k.SetStringValue(name, v)
	case []byte:
		err = k.SetBinaryValue(name, v)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func hasValue(path, name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetValue(name, nil)
	return err == nil
}

// Windows Settings writes 02… for on and 03… for off; the app reads that,
// and turning it on in the app clears the mark.
func TestStartupApproved(t *testing.T) {
	_, approved := testKeys(t)
	setValue(t, approved, "HealthTrail", []byte{3, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8})
	setValue(t, approved, "Other", []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if !approvedOff(registry.CURRENT_USER, approved, "HealthTrail") {
		t.Error("turned off in Settings, read as on")
	}
	if approvedOff(registry.CURRENT_USER, approved, "Other") || approvedOff(registry.CURRENT_USER, approved, "Missing") {
		t.Error("on, or never touched, read as off")
	}
	clearApproved(registry.CURRENT_USER, approved, "HealthTrail")
	if approvedOff(registry.CURRENT_USER, approved, "HealthTrail") {
		t.Error("still off after the app turned it on")
	}
}

// The exe moved: its old entry goes (and the caller registers the new
// place). Another copy's entry whose exe is still there, another program's,
// and one turned off in Settings stay.
func TestRemoveMoved(t *testing.T) {
	run, approved := testKeys(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "new place", "health-trail.exe")
	other := filepath.Join(dir, "other copy", "health-trail.exe")
	os.MkdirAll(filepath.Dir(other), 0o755)
	os.WriteFile(other, []byte("MZ"), 0o644)
	gone := filepath.Join(dir, "Downloads", "health-trail.exe")
	setValue(t, run, "HealthTrail", `"`+gone+`" -login`)
	setValue(t, run, "wails-app", `"`+other+`" -login`)
	setValue(t, run, "Something", `"`+filepath.Join(dir, "gone", "other.exe")+`"`)
	setValue(t, run, "OffInSettings", `"`+filepath.Join(dir, "old", "health-trail.exe")+`" -login`)
	setValue(t, approved, "OffInSettings", []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})

	if !removeMoved(registry.CURRENT_USER, run, approved, exe) {
		t.Fatal("the moved entry wasn't found")
	}
	if hasValue(run, "HealthTrail") {
		t.Error("the entry for the old place is still there")
	}
	for _, name := range []string{"wails-app", "Something", "OffInSettings"} {
		if !hasValue(run, name) {
			t.Errorf("%s was removed", name)
		}
	}
	if removeMoved(registry.CURRENT_USER, run, approved, exe) {
		t.Error("found a moved entry a second time")
	}
}

func TestCommandExe(t *testing.T) {
	for in, want := range map[string]string{
		`"C:\Program Files\HT\health-trail.exe" -login`: `C:\Program Files\HT\health-trail.exe`,
		`C:\HT\health-trail.exe -login`:                 `C:\HT\health-trail.exe`,
		`C:\HT\health-trail.exe`:                        `C:\HT\health-trail.exe`,
	} {
		if got := commandExe(in); got != want {
			t.Errorf("commandExe(%s) = %s", in, got)
		}
	}
}
