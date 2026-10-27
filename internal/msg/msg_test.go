package msg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestErrorfInBothLanguages(t *testing.T) {
	inner := New("需要数字", "needs a number")
	err := Errorf("%s「%s」：%v", "%s, %s: %v", Word{"体重", "Weight"}, Word{"数值", "Value"}, inner)
	if err.Error() != "体重「数值」：需要数字" {
		t.Fatalf("zh %q", err.Error())
	}
	if got := In(err, "en"); got != "Weight, Value: needs a number" {
		t.Fatalf("en %q", got)
	}
	if !errors.Is(err, inner) {
		t.Fatal("the cause is lost")
	}
	// wrapped by something that has no English: the inner English
	if got := In(fmt.Errorf("x: %w", inner), "en"); got != "needs a number" {
		t.Fatalf("wrapped %q", got)
	}
	// no English at all: as it is
	if got := In(errors.New("plain"), "en"); got != "plain" {
		t.Fatalf("plain %q", got)
	}
	if got := In(err, "zh"); got != err.Error() {
		t.Fatalf("zh %q", got)
	}
}

func TestJoin(t *testing.T) {
	if Join(nil, nil) != nil {
		t.Fatal("nil")
	}
	err := Join(New("甲", "A"), nil, New("乙", "B"))
	if err.Error() != "甲\n乙" || In(err, "en") != "A\nB" {
		t.Fatalf("%q %q", err.Error(), In(err, "en"))
	}
}

// Errors a page may show are written with this package (or as a
// store.ValidationError with En), so an English page doesn't show Chinese.
// mcpserver is left out: its errors are for the model, in Chinese.
func TestNoChineseOnlyErrors(t *testing.T) {
	han := `[\p{Han}]`
	bad := []*regexp.Regexp{
		regexp.MustCompile(`(errors\.New|fmt\.Errorf)\("[^"\n]*` + han),
		regexp.MustCompile(`ValidationError\{Msg: "[^"\n]*` + han + `[^}\n]*\}`),
	}
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "mcpserver" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, re := range bad {
				if m := re.FindString(line); m != "" && !strings.Contains(m, "En:") {
					t.Errorf("%s:%d has a Chinese-only error: %s", path, i+1, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
