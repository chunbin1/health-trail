package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
		{"name":"HealthTrail-1.2.0.zip","browser_download_url":"https://x/zip","size":20,"digest":"sha256:abcd"}]}`
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, body)
		case "/nozip":
			fmt.Fprint(w, strings.Replace(body, "HealthTrail-1.2.0.zip", "other.zip", 1))
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
	if r.Version != "1.2.0" || r.Notes != "- 新增：更新" || r.ZipURL != "https://x/zip" || r.ZipSize != 20 || r.ZipSHA256 != "abcd" || !r.Installable() {
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
	if err := in.download(context.Background(), ok, dst); err != nil || seen != int64(len(data)) {
		t.Fatalf("download = %v, progress %d", err, seen)
	}
	short := *ok
	short.ZipSize++
	if err := in.download(context.Background(), &short, dst); !errors.Is(err, ErrBadDownload) {
		t.Errorf("wrong size = %v, want ErrBadDownload", err)
	}
	tampered := *ok
	tampered.ZipSHA256 = strings.Repeat("0", 64)
	if err := in.download(context.Background(), &tampered, dst); !errors.Is(err, ErrBadDownload) {
		t.Errorf("wrong sha256 = %v, want ErrBadDownload", err)
	}
}

// fakeApp is an ad-hoc signed bundle, as make app builds.
func fakeApp(t *testing.T, dir, version string) string {
	t.Helper()
	app := filepath.Join(dir, "健康足迹.app")
	os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>com.chunbin1.healthtrail</string>
<key>CFBundleExecutable</key><string>health-trail</string>
<key>CFBundleShortVersionString</key><string>`+version+`</string>
</dict></plist>`), 0o644)
	os.WriteFile(filepath.Join(app, "Contents", "MacOS", "health-trail"), []byte("#!/bin/sh\n"), 0o755)
	if out, err := exec.Command("codesign", "--force", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Skipf("codesign: %v %s", err, out)
	}
	return app
}

func TestAdHocBuildDoesNotUpdate(t *testing.T) {
	app := fakeApp(t, t.TempDir(), "1.0.0")
	if team := Team(app); team != "" {
		t.Errorf("Team(ad hoc) = %q", team)
	}
	in := &Installer{App: app, BundleID: "com.chunbin1.healthtrail", Client: http.DefaultClient}
	if _, err := in.Install(context.Background(), &Release{Version: "1.0.1", ZipURL: "http://127.0.0.1:1/x"}); !errors.Is(err, ErrUnsigned) {
		t.Errorf("Install on an ad-hoc build = %v, want ErrUnsigned", err)
	}
}

// An ad-hoc signed app in the zip: refused by the signature check before
// anything is moved, and the stage is gone.
func TestUnsignedDownloadIsRefused(t *testing.T) {
	src := fakeApp(t, t.TempDir(), "1.0.1")
	zip := filepath.Join(t.TempDir(), "u.zip")
	if out, err := exec.Command("ditto", "-c", "-k", "--keepParent", src, zip).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v %s", err, out)
	}
	dir := t.TempDir()
	app := fakeApp(t, dir, "1.0.0")
	stage, _ := os.MkdirTemp(dir, stagePrefix)
	os.WriteFile(filepath.Join(stage, "update.zip"), nil, 0o644) // left by a crash
	unpacked := filepath.Join(stage, "new")
	exec.Command("ditto", "-x", "-k", zip, unpacked).Run()
	err := verify(context.Background(), filepath.Join(unpacked, "健康足迹.app"), "com.chunbin1.healthtrail", "XU4K6FP3HC", "1.0.1")
	if !errors.Is(err, ErrBadDownload) || !strings.Contains(err.Error(), "signature") {
		t.Errorf("verify(ad hoc) = %v, want a signature ErrBadDownload", err)
	}
	if v := bundleVersion(app); v != "1.0.0" {
		t.Errorf("bundleVersion = %q", v)
	}
	Cleanup(app)
	if left, _ := filepath.Glob(filepath.Join(dir, stagePrefix+"*")); len(left) != 0 {
		t.Errorf("Cleanup left %v", left)
	}
	if _, err := os.Stat(app); err != nil {
		t.Errorf("Cleanup touched the app: %v", err)
	}
}
