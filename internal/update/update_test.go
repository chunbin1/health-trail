package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		release, current string
		want             bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.12.3", true},
		{"1.10.0", "1.9.0", true}, // numbers, not text
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false}, // never back
		{"1.0.0", "1.0.0-test.2", true},
		{"1.0.0", "v1.0.0-test.2", true},
		// built from commits after a tag: that tag, not before it
		{"1.0.0", "1.0.0-3-gabc1234", false},
		{"1.0.1", "1.0.0-3-gabc1234", true},
		{"1.0.0", "1.0.0-test.2-3-gabc1234", true},
		// a build with no tag behind it, or no version at all
		{"1.0.0", "26c70bf", false},
		{"1.0.0", "", false},
		// a pre-release is never offered
		{"1.1.0-test.1", "1.0.0", false},
		{"v1.0.1", "1.0.0", false},
	} {
		if got := Newer(c.release, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.release, c.current, got, c.want)
		}
	}
}

func TestLatest(t *testing.T) {
	body := `{"tag_name":"v1.2.0","name":"健康足迹 1.2.0","body":"- 新增：更新","html_url":"https://github.com/x/y/releases/tag/v1.2.0",
		"published_at":"2026-11-01T10:00:00Z","assets":[
		{"name":"HealthTrail-1.2.0.dmg","browser_download_url":"https://x/dmg","size":10},
		{"name":"HealthTrail-1.2.0.zip","browser_download_url":"https://x/zip","size":20,"digest":"sha256:abcd"},
		{"name":"HealthTrail-1.2.0-windows.zip","browser_download_url":"https://x/wzip","size":30,"digest":"sha256:ef01"},
		{"name":"HealthTrail-1.2.0-windows.zip.sig","browser_download_url":"https://x/wsig","size":89}]}`
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, body)
		case "/nozip":
			s := strings.Replace(body, "HealthTrail-1.2.0-windows.zip", "other-win.zip", 1)
			fmt.Fprint(w, strings.Replace(s, "HealthTrail-1.2.0.zip", "other.zip", 1))
		case "/badtag":
			fmt.Fprint(w, strings.Replace(body, "v1.2.0", "nightly", 1))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	r, err := Latest(ctx, srv.Client(), srv.URL+"/ok")
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "1.2.0" || r.Notes != "- 新增：更新" || r.ZipURL != "https://x/zip" || r.ZipSize != 20 || r.ZipSHA256 != "abcd" || r.WinURL != "https://x/wzip" || r.WinSize != 30 || r.WinSHA256 != "ef01" || r.WinSigURL != "https://x/wsig" || !r.Installable() {
		t.Errorf("release = %+v", r)
	}
	// the tag message's first line is the release's name again
	if got := notes("健康足迹 1.2.0\r\n\r\n- 新增：更新\n", "健康足迹 1.2.0"); got != "- 新增：更新" {
		t.Errorf("notes without the title = %q", got)
	}
	if got := notes("第一行\n- 修复", "健康足迹 1.2.0"); got != "第一行\n- 修复" {
		t.Errorf("notes with another first line = %q", got)
	}
	if accept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", accept)
	}
	// a release from before the zip: found, not installable
	if r, err := Latest(ctx, srv.Client(), srv.URL+"/nozip"); err != nil || r.Installable() {
		t.Errorf("no zip: %+v, %v", r, err)
	}
	if _, err := Latest(ctx, srv.Client(), srv.URL+"/badtag"); err == nil {
		t.Error("a tag that isn't a version was taken")
	}
	// a private repository, or none released yet
	if _, err := Latest(ctx, srv.Client(), srv.URL+"/missing"); !errors.Is(err, ErrNoRelease) {
		t.Errorf("404 = %v, want ErrNoRelease", err)
	}
}

func TestDownloadChecks(t *testing.T) {
	data := []byte("not really a zip")
	sum := sha256.Sum256(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	var seen int64
	in := &Installer{Client: srv.Client(), Progress: func(done, total int64) { seen = done }}
	dst := filepath.Join(t.TempDir(), "u.zip")
	ok := &Release{ZipURL: srv.URL, ZipSize: int64(len(data)), ZipSHA256: hex.EncodeToString(sum[:])}
	if err := in.download(context.Background(), ok.ZipURL, ok.ZipSize, ok.ZipSHA256, dst); err != nil || seen != int64(len(data)) {
		t.Fatalf("download = %v, progress %d", err, seen)
	}
	short := *ok
	short.ZipSize++
	if err := in.download(context.Background(), short.ZipURL, short.ZipSize, short.ZipSHA256, dst); !errors.Is(err, ErrBadDownload) {
		t.Errorf("wrong size = %v, want ErrBadDownload", err)
	}
	tampered := *ok
	tampered.ZipSHA256 = strings.Repeat("0", 64)
	if err := in.download(context.Background(), tampered.ZipURL, tampered.ZipSize, tampered.ZipSHA256, dst); !errors.Is(err, ErrBadDownload) {
		t.Errorf("wrong sha256 = %v, want ErrBadDownload", err)
	}
}

func TestWindowsZipPayload(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "u.zip")
	writeZip(t, zipPath, map[string]string{
		"health-trail.exe": "MZ",
		"version.txt":      "1.2.0\n",
		"../escape.exe":    "no",
	})
	dst := filepath.Join(dir, "out")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unpackZip(mustRead(t, zipPath), dst); !errors.Is(err, ErrBadDownload) {
		t.Fatalf("zip slip = %v", err)
	}

	writeZip(t, zipPath, map[string]string{"health-trail.exe": "MZ", "version.txt": "1.2.0\n"})
	os.RemoveAll(dst)
	os.Mkdir(dst, 0o755)
	if err := unpackZip(mustRead(t, zipPath), dst); err != nil {
		t.Fatal(err)
	}
	exe, err := windowsPayload(dst, "1.2.0")
	if err != nil || filepath.Base(exe) != "health-trail.exe" {
		t.Fatalf("payload = %s %v", exe, err)
	}
	if _, err := windowsPayload(dst, "9.9.9"); !errors.Is(err, ErrBadDownload) {
		t.Fatalf("wrong version = %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// withTestKey signs with a key of the test's own in place of the release key.
func withTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	old := publicKeys
	publicKeys = []string{base64.StdEncoding.EncodeToString(pub)}
	t.Cleanup(func() { publicKeys = old })
	return priv
}

func TestCheckSignature(t *testing.T) {
	key := withTestKey(t)
	data := []byte("PK zip bytes")
	sig := Sign(key, data)
	if err := CheckSignature(data, sig); err != nil {
		t.Fatalf("our signature = %v", err)
	}
	// a signature file written on Windows may end in CRLF
	if err := CheckSignature(data, append(bytes.TrimSpace(sig), '\r', '\n')); err != nil {
		t.Fatalf("CRLF = %v", err)
	}
	_, other, _ := ed25519.GenerateKey(nil)
	for name, c := range map[string]struct{ data, sig []byte }{
		"changed zip":     {[]byte("PK zip bytez"), sig},
		"another key":     {data, Sign(other, data)},
		"not base64":      {data, []byte("not a signature")},
		"empty signature": {data, nil},
	} {
		if err := CheckSignature(c.data, c.sig); !errors.Is(err, ErrBadDownload) {
			t.Errorf("%s = %v, want ErrBadDownload", name, err)
		}
	}
}

// The key release.yml signs with must match the one built in: a release
// signed with anything else would never install.
func TestBuiltInKeyIsValid(t *testing.T) {
	if len(publicKeys) == 0 {
		t.Fatal("no public key")
	}
	for _, k := range publicKeys {
		if b, err := base64.StdEncoding.DecodeString(k); err != nil || len(b) != ed25519.PublicKeySize {
			t.Errorf("public key %q: %v", k, err)
		}
	}
}
