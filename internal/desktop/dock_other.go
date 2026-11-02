//go:build !darwin

package desktop

// The menu-bar icon is the system tray. There is no Dock to hide, and no
// login-item launch flag to read; the window opens at start.
func launchedAtLogin() bool { return false }

func setInDock(bool) {}
