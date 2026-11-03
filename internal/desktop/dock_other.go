//go:build !darwin

package desktop

// The menu-bar icon is the system tray, and there is no Dock to hide. A
// start at login says so with -login (shell.setAutostart puts it there).
func launchedAtLogin() bool { return OpenedAtLogin }

func setInDock(bool) {}
