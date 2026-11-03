//go:build windows

package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsignedExeDoesNotUpdate(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "health-trail.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := &Installer{App: exe}
	if _, err := in.Check(); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("Check(not a release build) = %v, want ErrUnsigned", err)
	}
}

// A release build takes a zip signed with our key: the new exe is in place,
// the old one waits in the stage. A zip signed with another key, or one
// changed after signing, leaves everything as it was.
func TestInstallSignedZip(t *testing.T) {
	key := withTestKey(t)
	old := Channel
	Channel = "release"
	t.Cleanup(func() { Channel = old })

	src := t.TempDir()
	zipPath := filepath.Join(src, "u.zip")
	writeZip(t, zipPath, map[string]string{"health-trail.exe": "MZ new", "version.txt": "1.2.0\r\n"})
	zipData := mustRead(t, zipPath)
	good := Sign(key, zipData)
	_, other, _ := ed25519.GenerateKey(nil)

	for _, c := range []struct {
		name    string
		zip     []byte
		sig     []byte
		wantErr error
	}{
		{"another key", zipData, Sign(other, zipData), ErrBadDownload},
		{"changed after signing", append(bytes.Clone(zipData), 0), good, ErrBadDownload},
		{"signed by us", zipData, good, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, ".sig") {
					w.Write(c.sig)
					return
				}
				w.Write(c.zip)
			}))
			defer srv.Close()
			dir := t.TempDir()
			app := filepath.Join(dir, "health-trail.exe")
			os.WriteFile(app, []byte("MZ old"), 0o644)
			in := &Installer{App: app, Client: srv.Client()}
			r := &Release{Version: "1.2.0", WinURL: srv.URL + "/z.zip", WinSigURL: srv.URL + "/z.zip.sig"}
			stage, err := in.Install(context.Background(), r)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("Install = %v, want %v", err, c.wantErr)
			}
			got, _ := os.ReadFile(app)
			if c.wantErr != nil {
				if string(got) != "MZ old" {
					t.Errorf("the app was changed to %q", got)
				}
				if left, _ := filepath.Glob(filepath.Join(dir, stagePrefix+"*")); len(left) != 0 {
					t.Errorf("stage left behind: %v", left)
				}
				return
			}
			if string(got) != "MZ new" {
				t.Errorf("app = %q, want the new exe", got)
			}
			if b, _ := os.ReadFile(filepath.Join(stage, "old.exe")); string(b) != "MZ old" {
				t.Errorf("old exe in the stage = %q", b)
			}
		})
	}
}

// The relaunch command line: nothing at all when the app had no arguments
// (Start-Process refuses an empty list), and a data folder with a space or
// a quote stays one argument.
func TestRelaunchScript(t *testing.T) {
	s := relaunchScript(42, `C:\Apps\Health Trail\health-trail.exe`, `C:\Apps\Health Trail\.health-trail-update-1`, nil)
	if strings.Contains(s, "ArgumentList") {
		t.Errorf("no arguments but:\n%s", s)
	}
	if !strings.Contains(s, `$exe = 'C:\Apps\Health Trail\health-trail.exe'`) || !strings.Contains(s, "Get-Process -Id 42 ") {
		t.Errorf("script:\n%s", s)
	}
	s = relaunchScript(42, `C:\a.exe`, `C:\s`, []string{"-data", `C:\Users\Jane O'Neil\Health Data`, "-port", "8734"})
	want := `-ArgumentList '-data "C:\Users\Jane O''Neil\Health Data" -port 8734'`
	if !strings.Contains(s, want) {
		t.Errorf("arguments: want %s in\n%s", want, s)
	}
	if got := psQuote("a’b"); got != "'a’’b'" {
		t.Errorf("psQuote = %s", got)
	}
}
