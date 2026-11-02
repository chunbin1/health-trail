//go:build windows

package update

import (
	"os"
	"syscall"
)

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".health-trail-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
