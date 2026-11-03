//go:build windows

package desktop

import "testing"

func TestRevealCmdLine(t *testing.T) {
	got := revealCmdLine(`C:\Users\Jane Doe\AppData\Local\Temp\health-trail-open\3\体检, 2026.pdf`)
	want := `explorer /select,"C:\Users\Jane Doe\AppData\Local\Temp\health-trail-open\3\体检, 2026.pdf"`
	if got != want {
		t.Errorf("command line = %s, want %s", got, want)
	}
}
