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

// Dictation comes in phrase by phrase: every recognized phrase stays, the
// one still being heard is shown after them and replaced as it changes.
func TestHeardKeepsEveryPhrase(t *testing.T) {
	for _, c := range []struct {
		sep  string
		want string
	}{{"", "今天头疼吃了布洛芬"}, {" ", "headache today took ibuprofen"}} {
		e := &winEar{sep: c.sep}
		words := []string{"今天头疼", "吃了布洛芬"}
		if c.sep != "" {
			words = []string{"headache today", "took ibuprofen"}
		}
		e.heard(words[0][:3], false)
		e.heard(words[0], true)
		e.heard(words[1][:3], false)
		if got := e.View().Text; got != words[0]+c.sep+words[1][:3] {
			t.Fatalf("while hearing the second phrase = %q", got)
		}
		e.heard(words[1], true)
		if got := e.View().Text; got != c.want {
			t.Fatalf("text = %q, want %q", got, c.want)
		}
	}
}
