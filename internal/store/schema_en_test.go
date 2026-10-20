package store

import (
	"testing"
	"unicode"
)

// Every Chinese string the UI shows from the schema needs its English in
// schema_en.go, or the English UI would show it untranslated.
func TestEnglishCoversSchema(t *testing.T) {
	need := func(where, s string) {
		t.Helper()
		if !hasHan(s) {
			return
		}
		if English[s] == "" {
			t.Errorf("%s「%s」在 schema_en.go 的 English 里没有英文", where, s)
		}
	}
	for _, e := range Entities {
		need(e.Name+" 的名称", e.Label)
		if EnglishNouns[e.Name] == "" {
			t.Errorf("%s 在 schema_en.go 的 EnglishNouns 里没有英文", e.Name)
		}
		for _, f := range e.Fields {
			where := e.Name + "." + f.Name
			need(where+" 的 Label", f.Label)
			need(where+" 的 Hint", f.Hint)
			need(where+" 的 Unit", f.Unit)
			for _, o := range f.Options {
				need(where+" 的选项", o)
			}
		}
	}
	for _, m := range MetricTypes {
		need("MetricTypes", m)
	}
	for _, u := range MetricUnits {
		need("MetricUnits", u)
	}
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
