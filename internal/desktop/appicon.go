package desktop

import _ "embed"

// The same picture as the Mac app icon. Windows uses it for the window
// and the taskbar when the exe has no icon resource numbered 3. The file
// icon in Explorer comes from rsrc_windows_*.syso next to main.go.
//
//go:embed appicon.png
var appIcon []byte
