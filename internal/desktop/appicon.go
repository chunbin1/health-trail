package desktop

import _ "embed"

// The Windows window and taskbar icon. frontend/public/icon.svg is the
// drawing, build/AppIcon.icns is the Mac app icon, and this PNG is what
// Windows shows. After replacing it, run `make win-icon` so Explorer's
// file icon (rsrc_windows_*.syso next to main.go) matches.
//
//go:embed appicon.png
var appIcon []byte
