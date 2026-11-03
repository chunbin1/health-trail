package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// stagePrefix names the folder next to the app where an update is unpacked
// and the old copy waits to be deleted. A leftover one (a crash midway) is
// removed by Cleanup.
const stagePrefix = ".health-trail-update-"

// Why an update can't be installed from the app; the desktop layer says
// these in the person's language.
var (
	// a build without a Developer ID signature: one made with make app, or
	// a test build. It may be replaced only by hand.
	ErrUnsigned = errors.New("this build isn't signed with a Developer ID")
	// run from the dmg, from a folder macOS moved it out of (App
	// Translocation: opened from Downloads without moving it), or from a
	// folder this user can't write to
	ErrNotMovable = errors.New("the app's folder can't be written to")
	// the download isn't the app it should be: not signed by the same team,
	// not notarized, another version, or damaged
	ErrBadDownload = errors.New("the download didn't pass the checks")
)

// Installer replaces the app at App with a release.
type Installer struct {
	App      string // the running .app
	BundleID string
	Client   *http.Client
	// Progress is told how much of the zip has arrived
	Progress func(done, total int64)
}

func errorsNewNoZip() error { return errors.New("the release has no zip to install") }

const (
	windowsExe  = "health-trail.exe"
	versionFile = "version.txt"
)

func (in *Installer) download(ctx context.Context, url string, size int64, sha, dst string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "health-trail")
	resp, err := in.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	sum := sha256.New()
	pw := &progress{total: size, report: in.Progress}
	n, err := io.Copy(io.MultiWriter(f, sum, pw), resp.Body)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if size > 0 && n != size {
		return fmt.Errorf("%w: got %d bytes of %d", ErrBadDownload, n, size)
	}
	if sha != "" && !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), sha) {
		return fmt.Errorf("%w: sha256 differs", ErrBadDownload)
	}
	return f.Close()
}

type progress struct {
	done, total int64
	report      func(done, total int64)
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.report != nil {
		p.report(p.done, p.total)
	}
	return len(b), nil
}

// unpackZip extracts src into dst. An entry that would write outside dst is refused.
func unpackZip(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("%w: unzip: %v", ErrBadDownload, err)
	}
	defer zr.Close()
	root, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	var total int64
	for _, f := range zr.File {
		name := filepath.Clean(f.Name)
		if name == "." || strings.HasPrefix(name, "..") || filepath.IsAbs(f.Name) {
			return fmt.Errorf("%w: zip entry %q", ErrBadDownload, f.Name)
		}
		target := filepath.Join(root, name)
		rel, err := filepath.Rel(root, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: zip entry %q", ErrBadDownload, f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%w: unzip: %v", ErrBadDownload, err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		n, err := io.Copy(out, io.LimitReader(rc, 512<<20))
		rc.Close()
		cerr := out.Close()
		total += n
		if err != nil || cerr != nil || total > 512<<20 {
			return fmt.Errorf("%w: unzip", ErrBadDownload)
		}
	}
	return nil
}

// windowsPayload is the exe in a Windows update zip, when version.txt is the
// release's version.
func windowsPayload(dir, version string) (string, error) {
	exe := filepath.Join(dir, windowsExe)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("%w: no %s", ErrBadDownload, windowsExe)
	}
	b, err := os.ReadFile(filepath.Join(dir, versionFile))
	if err != nil {
		return "", fmt.Errorf("%w: no version", ErrBadDownload)
	}
	if got := strings.TrimSpace(string(b)); got != version {
		return "", fmt.Errorf("%w: the app inside is version %q, the release %q", ErrBadDownload, got, version)
	}
	return exe, nil
}

// Cleanup removes what an update cut short left next to app.
func Cleanup(app string) {
	if app == "" {
		return
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(app), stagePrefix+"*"))
	for _, d := range left {
		os.RemoveAll(d)
	}
}
