//go:build unix

package update

import "syscall"

func dirWritable(dir string) bool {
	return syscall.Access(dir, 2 /* W_OK */) == nil
}

func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
