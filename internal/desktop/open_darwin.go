//go:build darwin

package desktop

import "os/exec"

func openWith(args ...string) error {
	return exec.Command("open", args...).Run()
}
