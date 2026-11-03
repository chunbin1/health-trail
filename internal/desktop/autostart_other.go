//go:build !windows

package desktop

// Only Windows keeps a separate "turned off in Settings" mark, and only
// there does the login entry name the exe's path.

func startupDisabled(string) bool { return false }

func clearStartupDisabled(string) {}

func movedAutostart(string) bool { return false }
