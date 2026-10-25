package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
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

// Bundle is the .app the running binary is in (Contents/MacOS/<binary>),
// or "" outside one.
func Bundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	app := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if filepath.Base(filepath.Dir(exe)) != "MacOS" || !strings.HasSuffix(app, ".app") {
		return ""
	}
	return app
}

var teamRe = regexp.MustCompile(`(?m)^TeamIdentifier=([A-Z0-9]{10})$`)

// Team is the Developer ID team that signed app, or "" for an ad-hoc or
// missing signature.
func Team(app string) string {
	out, _ := exec.Command("codesign", "-dv", "--verbose=2", app).CombinedOutput()
	if !strings.Contains(string(out), "Authority=Developer ID Application:") {
		return ""
	}
	if m := teamRe.FindSubmatch(out); m != nil {
		return string(m[1])
	}
	return ""
}

// Check says why the app at in.App can't be updated in place, or nil when
// it can. It returns the team the new copy must be signed by.
func (in *Installer) Check() (string, error) {
	team := Team(in.App)
	if team == "" {
		return "", ErrUnsigned
	}
	if strings.Contains(in.App, "/AppTranslocation/") || strings.HasPrefix(in.App, "/Volumes/") {
		return "", ErrNotMovable
	}
	if syscall.Access(filepath.Dir(in.App), 2 /* W_OK */) != nil {
		return "", ErrNotMovable
	}
	return team, nil
}

// Install downloads r, checks it and swaps it in for in.App. The running app
// keeps working from the old copy, moved into the returned folder; Relaunch
// deletes that folder and opens the new copy once the app has quit.
func (in *Installer) Install(ctx context.Context, r *Release) (string, error) {
	team, err := in.Check()
	if err != nil {
		return "", err
	}
	if !r.Installable() {
		return "", errors.New("the release has no zip to install")
	}
	// next to the app, so the swap is two renames on one volume
	stage, err := os.MkdirTemp(filepath.Dir(in.App), stagePrefix)
	if err != nil {
		return "", ErrNotMovable
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(stage)
		}
	}()
	zip := filepath.Join(stage, "update.zip")
	if err := in.download(ctx, r, zip); err != nil {
		return "", err
	}
	unpacked := filepath.Join(stage, "new")
	if out, err := exec.CommandContext(ctx, "ditto", "-x", "-k", zip, unpacked).CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w: unzip: %v %s", ErrBadDownload, err, out)
	}
	apps, _ := filepath.Glob(filepath.Join(unpacked, "*.app"))
	if len(apps) != 1 {
		return "", fmt.Errorf("%w: %d apps in the zip", ErrBadDownload, len(apps))
	}
	if err := verify(ctx, apps[0], in.BundleID, team, r.Version); err != nil {
		return "", err
	}
	old := filepath.Join(stage, "old.app")
	if err := os.Rename(in.App, old); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotMovable, err)
	}
	if err := os.Rename(apps[0], in.App); err != nil {
		if rerr := os.Rename(old, in.App); rerr != nil {
			// the app is now only in the stage: keep it, and say where
			ok = true
			return "", fmt.Errorf("putting the new copy in place: %v; the old one is in %s: %v", err, old, rerr)
		}
		return "", fmt.Errorf("%w: %v", ErrNotMovable, err)
	}
	os.Remove(zip)
	ok = true
	return stage, nil
}

func (in *Installer) download(ctx context.Context, r *Release, dst string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", r.ZipURL, nil)
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
	pw := &progress{total: r.ZipSize, report: in.Progress}
	n, err := io.Copy(io.MultiWriter(f, sum, pw), resp.Body)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if r.ZipSize > 0 && n != r.ZipSize {
		return fmt.Errorf("%w: got %d bytes of %d", ErrBadDownload, n, r.ZipSize)
	}
	if r.ZipSHA256 != "" && !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), r.ZipSHA256) {
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

// verify accepts app only if it's signed with a Developer ID of team, has
// our bundle id, is notarized, and is the version the release says.
func verify(ctx context.Context, app, bundleID, team, version string) error {
	// Apple's designated requirement for Developer ID apps, pinned to the team
	req := fmt.Sprintf(`=identifier "%s" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "%s"`, bundleID, team)
	if out, err := exec.CommandContext(ctx, "codesign", "--verify", "--deep", "--strict", "-R", req, app).CombinedOutput(); err != nil {
		return fmt.Errorf("%w: signature: %s", ErrBadDownload, strings.TrimSpace(string(out)))
	}
	if out, err := exec.CommandContext(ctx, "spctl", "--assess", "--type", "execute", app).CombinedOutput(); err != nil {
		return fmt.Errorf("%w: not notarized: %s", ErrBadDownload, strings.TrimSpace(string(out)))
	}
	if got := bundleVersion(app); got != version {
		return fmt.Errorf("%w: the app inside is version %q, the release %q", ErrBadDownload, got, version)
	}
	return nil
}

var shortVersionRe = regexp.MustCompile(`<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`)

func bundleVersion(app string) string {
	b, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return ""
	}
	if m := shortVersionRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

// Relaunch starts a helper that waits for this process to exit, deletes
// stage (the old copy) and opens app with args. The caller quits next.
func Relaunch(app, stage string, args []string) error {
	const script = `pid=$1 stage=$2 app=$3; shift 3
while kill -0 "$pid" 2>/dev/null; do sleep 0.2; done
rm -rf "$stage"
exec open -n "$app" --args "$@"`
	cmd := exec.Command("/bin/sh", append([]string{"-c", script, "sh", strconv.Itoa(os.Getpid()), stage, app}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
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
