//go:build windows

package desktop

import (
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// registerProtocol makes healthtrail:// open this exe. It is per-user, so it
// does not need an administrator. A later start rewrites the command, so
// moving the exe keeps the links pointed at it.
func registerProtocol() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("登记 healthtrail 链接失败：%v", err)
		return
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	if err := installURLScheme(registry.CURRENT_USER, `Software\Classes\healthtrail`, exe); err != nil {
		log.Printf("登记 healthtrail 链接失败：%v", err)
	}
}

func installURLScheme(root registry.Key, path, exe string) error {
	cmd, err := protocolCommand(exe)
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(root, path, registry.SET_VALUE|registry.CREATE_SUB_KEY|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("", "URL:Health Trail"); err != nil {
		return err
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}
	ck, _, err := registry.CreateKey(k, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer ck.Close()
	return ck.SetStringValue("", cmd)
}
