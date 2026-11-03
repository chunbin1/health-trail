//go:build windows

package desktop

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestURLSchemeRegistration(t *testing.T) {
	path := `Software\HealthTrailTest\protocol`
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, path+`\shell\open\command`)
		registry.DeleteKey(registry.CURRENT_USER, path+`\shell\open`)
		registry.DeleteKey(registry.CURRENT_USER, path+`\shell`)
		registry.DeleteKey(registry.CURRENT_USER, path)
	})
	exe := `C:\Program Files\HealthTrail\health-trail.exe`
	if err := installURLScheme(registry.CURRENT_USER, path, exe); err != nil {
		t.Fatal(err)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, path+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	got, _, err := k.GetStringValue("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := protocolCommand(exe)
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}
