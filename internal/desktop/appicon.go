package desktop

import (
	_ "embed"
	"runtime"
)

// The Windows window and taskbar icon. Corners are transparent: the Mac
// icon file paints them white, and Windows would show that as a white
// square. frontend/public/icon.svg is the drawing, build/AppIcon.icns is
// the Mac app icon. After replacing this PNG, run `make win-icon`. That
// writes rsrc_windows_*.syso with icon id 3, which is the id the window
// loads; Explorer uses the same resource.
//
//go:embed appicon.png
var appIcon []byte

// windowsIcon is only for Windows. On Mac the bundle icon is AppIcon.icns;
// passing this PNG there replaces it.
func windowsIcon() []byte {
	if runtime.GOOS == "windows" {
		return appIcon
	}
	return nil
}
