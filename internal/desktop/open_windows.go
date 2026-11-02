//go:build windows

package desktop

import (
	"fmt"
	"os/exec"
)

// openWith runs the same arguments attachopen passes to `open` on the Mac:
// the file itself, or "-R" and the file to show it in a folder.
func openWith(args ...string) error {
	var path string
	reveal := false
	switch {
	case len(args) == 1:
		path = args[0]
	case len(args) == 2 && args[0] == "-R":
		reveal, path = true, args[1]
	default:
		return fmt.Errorf("open: %q", args)
	}
	if reveal {
		// explorer.exe exits non-zero even when it shows the file
		exec.Command("explorer", "/select,"+path).Run()
		return nil
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Run()
}
