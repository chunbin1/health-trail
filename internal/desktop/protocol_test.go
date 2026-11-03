package desktop

import "testing"

func TestProtocolCommand(t *testing.T) {
	got, err := protocolCommand(`C:\Program Files\HealthTrail\health-trail.exe`)
	if err != nil || got != `"C:\Program Files\HealthTrail\health-trail.exe" "%1"` {
		t.Fatalf("command = %q %v", got, err)
	}
	if _, err := protocolCommand(`C:\bad"quote\health-trail.exe`); err == nil {
		t.Fatal("a quote in the path was accepted")
	}
}
