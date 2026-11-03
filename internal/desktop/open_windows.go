//go:build windows

package desktop

import (
	"fmt"
	"os/exec"
	"syscall"
)

// openWith runs the same arguments attachopen passes to `open` on the Mac:
// the file itself, or "-R" and the file to show it in a folder.
func openWith(args ...string) error {
	switch {
	case len(args) == 1:
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", args[0]).Run()
	case len(args) == 2 && args[0] == "-R":
		cmd := exec.Command("explorer")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: revealCmdLine(args[1])}
		// explorer.exe exits non-zero even when it shows the file
		cmd.Run()
		return nil
	default:
		return fmt.Errorf("open: %q", args)
	}
}

// revealCmdLine is explorer's command line to select path. Explorer reads
// `/select,` itself and wants the quotes around the path only: Go's usual
// quoting would wrap the whole `/select,…` argument, and a path with a space
// (C:\Users\Jane Doe\…) would open Documents instead.
func revealCmdLine(path string) string {
	return `explorer /select,"` + path + `"`
}
