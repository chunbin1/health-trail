//go:build windows

package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Where the login entry lives, and where Windows notes that the person
// turned it off in Settings → Apps → Startup (or Task Manager): it leaves
// the entry in Run and marks it disabled here instead.
const (
	runKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

// startupDisabled says whether Windows has the login entry named name
// turned off. The first byte of the value is even when on, odd when off.
func startupDisabled(name string) bool {
	return approvedOff(registry.CURRENT_USER, approvedKey, name)
}

func approvedOff(root registry.Key, path, name string) bool {
	if name == "" {
		return false
	}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue(name)
	return err == nil && len(b) > 0 && b[0]&1 == 1
}

// clearStartupDisabled forgets an earlier "off" from Windows Settings, so
// turning the switch on in the app really starts it at login again.
func clearStartupDisabled(name string) {
	clearApproved(registry.CURRENT_USER, approvedKey, name)
}

func clearApproved(root registry.Key, path, name string) {
	if name == "" {
		return
	}
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	k.DeleteValue(name)
}

// movedAutostart finds a login entry left by this app at a place the exe
// isn't any more (it was moved, or unzipped somewhere else and the old
// copy deleted), removes it and says so. One whose exe is still there
// belongs to another copy and stays.
// An entry the person turned off in Windows Settings is left as it is.
func movedAutostart(exe string) bool {
	return removeMoved(registry.CURRENT_USER, runKey, approvedKey, exe)
}

func removeMoved(root registry.Key, path, approved, exe string) bool {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return false
	}
	found := false
	for _, name := range names {
		cmd, _, err := k.GetStringValue(name)
		if err != nil {
			continue
		}
		old := commandExe(cmd)
		if !strings.EqualFold(filepath.Base(old), filepath.Base(exe)) || strings.EqualFold(old, exe) {
			continue
		}
		if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) || approvedOff(root, approved, name) {
			continue
		}
		if k.DeleteValue(name) == nil {
			found = true
		}
	}
	return found
}

// commandExe is the program in a Windows command line: the quoted part, or
// up to the first space.
func commandExe(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, `"`) {
		if end := strings.IndexByte(cmd[1:], '"'); end >= 0 {
			return cmd[1 : 1+end]
		}
		return cmd[1:]
	}
	if i := strings.IndexAny(cmd, " \t"); i >= 0 {
		return cmd[:i]
	}
	return cmd
}
