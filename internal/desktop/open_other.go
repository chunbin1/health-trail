//go:build !darwin && !windows

package desktop

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// openWith follows `open` on the Mac with the desktop's opener: the file
// itself, or its folder for "-R" (there is no common way to select it).
func openWith(args ...string) error {
	switch {
	case len(args) == 1:
		return exec.Command("xdg-open", args[0]).Run()
	case len(args) == 2 && args[0] == "-R":
		return exec.Command("xdg-open", filepath.Dir(args[1])).Run()
	default:
		return fmt.Errorf("open: %q", args)
	}
}
