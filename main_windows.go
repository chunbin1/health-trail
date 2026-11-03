//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	// Built as a window program, so double-clicking does not open a console.
	// An AI that starts `health-trail mcp` already passed pipes in; leave
	// those alone. A terminal has no pipes, so attach to it and print there.
	stdout, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err == nil && stdout != 0 && stdout != windows.InvalidHandle {
		return
	}
	r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole").Call(^uintptr(0))
	if r == 0 {
		return
	}
	in, _ := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	out, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	errh, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	os.Stdin = os.NewFile(uintptr(in), "stdin")
	os.Stdout = os.NewFile(uintptr(out), "stdout")
	os.Stderr = os.NewFile(uintptr(errh), "stderr")
}
