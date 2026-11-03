//go:build darwin

package update

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
