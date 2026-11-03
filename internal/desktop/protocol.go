package desktop

import (
	"errors"
	"strings"
)

// protocolCommand is the shell open command for a healthtrail:// link.
// Windows quotes the exe and passes the URL as %1.
func protocolCommand(exe string) (string, error) {
	if exe == "" || strings.Contains(exe, `"`) {
		return "", errors.New("executable path can't be registered")
	}
	return `"` + exe + `" "%1"`, nil
}
