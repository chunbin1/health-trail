//go:build windows

package desktop

import "testing"

func TestSpeechSinkCallback(t *testing.T) {
	// NewCallback panics on a bad signature; building the sink is enough.
	s := newSpeechSink(&winEar{})
	if s.lpVtbl == nil || s.lpVtbl.invoke == 0 {
		t.Fatal("speech sink callback was not installed")
	}
}

func TestSpeechLangHex(t *testing.T) {
	if h, ok := speechLangHex("zh-CN"); !ok || h != "804" {
		t.Fatalf("zh-CN = %q %v", h, ok)
	}
	if h, ok := speechLangHex("en-US"); !ok || h != "409" {
		t.Fatalf("en-US = %q %v", h, ok)
	}
	if _, ok := speechLangHex("fr-FR"); ok {
		t.Fatal("fr-FR was accepted")
	}
}
