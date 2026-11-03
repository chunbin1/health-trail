//go:build windows

package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".health-trail-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func detachAttr() *syscall.SysProcAttr {
	// DETACHED_PROCESS (0x8) lets the helper outlive this process, the way
	// Setsid does on Mac. It is not in the syscall package.
	const detached = 0x00000008
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detached}
}

// Bundle is the running health-trail.exe, or "" for a build under another name.
func Bundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	if !strings.EqualFold(filepath.Base(exe), windowsExe) {
		return ""
	}
	return exe
}

// Check says why this exe can't be replaced, or nil when it can. Only a
// copy release.yml built updates itself; there is no certificate to ask,
// the download is checked against our own key instead (sign.go).
func (in *Installer) Check() (string, error) {
	if Channel != "release" {
		return "", ErrUnsigned
	}
	if !dirWritable(filepath.Dir(in.App)) {
		return "", ErrNotMovable
	}
	return "", nil
}

// Install downloads r and its signature, checks them and swaps the new exe
// in for in.App. Windows lets a running exe be renamed, not deleted: the old
// one moves into the returned folder, which Relaunch deletes once this
// process has quit.
func (in *Installer) Install(ctx context.Context, r *Release) (string, error) {
	if _, err := in.Check(); err != nil {
		return "", err
	}
	if !r.Installable() {
		return "", errorsNewNoZip()
	}
	url, size, sha := r.Payload()
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
	if err := in.download(ctx, url, size, sha, zip); err != nil {
		return "", err
	}
	sig, err := in.fetchSig(ctx, r.WinSigURL)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(zip)
	if err != nil {
		return "", err
	}
	if err := CheckSignature(data, sig); err != nil {
		return "", err
	}
	unpacked := filepath.Join(stage, "new")
	if err := os.MkdirAll(unpacked, 0o755); err != nil {
		return "", err
	}
	if err := unpackZip(data, unpacked); err != nil {
		return "", err
	}
	exe, err := windowsPayload(unpacked, r.Version)
	if err != nil {
		return "", err
	}
	old := filepath.Join(stage, "old.exe")
	if err := os.Rename(in.App, old); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotMovable, err)
	}
	if err := os.Rename(exe, in.App); err != nil {
		if rerr := os.Rename(old, in.App); rerr != nil {
			ok = true
			return "", fmt.Errorf("putting the new copy in place: %v; the old one is in %s: %v", err, old, rerr)
		}
		return "", fmt.Errorf("%w: %v", ErrNotMovable, err)
	}
	os.Remove(zip)
	ok = true
	return stage, nil
}

// Relaunch starts a helper that waits for this process to exit, deletes
// stage (the old exe) and starts the new exe with args. The caller quits next.
func Relaunch(exe, stage string, args []string) error {
	script := relaunchScript(os.Getpid(), exe, stage, args)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = detachAttr()
	return cmd.Start()
}

// relaunchScript is the PowerShell for Relaunch. The arguments go to
// Start-Process as one command line, quoted the Windows way: given as an
// array, Start-Process joins them with spaces and a path with a space
// splits; given an empty array, it refuses to start at all.
func relaunchScript(pid int, exe, stage string, args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	start := "Start-Process -FilePath $exe"
	if len(args) > 0 {
		start += " -ArgumentList " + psQuote(strings.Join(quoted, " "))
	}
	return fmt.Sprintf(`$exe = %s
while (Get-Process -Id %d -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }
Remove-Item -LiteralPath %s -Recurse -Force -ErrorAction SilentlyContinue
%s
`, psQuote(exe), pid, psQuote(stage), start)
}

// psQuote is s as a PowerShell literal string. PowerShell also ends one at
// the typographic single quotes, so those are doubled too.
func psQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '\u2018', '\u2019', '\u201a', '\u201b':
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}
