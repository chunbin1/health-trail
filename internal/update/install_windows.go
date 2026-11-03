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

// Check says why this exe can't be replaced, or nil when it can. The string
// is the Authenticode signer's certificate thumbprint.
func (in *Installer) Check() (string, error) {
	pub, err := publisher(in.App)
	if err != nil || pub == "" {
		return "", ErrUnsigned
	}
	if !dirWritable(filepath.Dir(in.App)) {
		return "", ErrNotMovable
	}
	return pub, nil
}

func (in *Installer) Install(ctx context.Context, r *Release) (string, error) {
	pub, err := in.Check()
	if err != nil {
		return "", err
	}
	url, size, sha := r.Payload()
	if url == "" {
		return "", errorsNewNoZip()
	}
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
	unpacked := filepath.Join(stage, "new")
	if err := os.MkdirAll(unpacked, 0o755); err != nil {
		return "", err
	}
	if err := unpackZip(zip, unpacked); err != nil {
		return "", err
	}
	exe, err := windowsPayload(unpacked, r.Version)
	if err != nil {
		return "", err
	}
	got, err := publisher(exe)
	if err != nil || !strings.EqualFold(got, pub) {
		return "", fmt.Errorf("%w: signature", ErrBadDownload)
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

// publisher is the signer's certificate thumbprint when the Authenticode
// signature is valid.
func publisher(path string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`$s = Get-AuthenticodeSignature -LiteralPath $env:HEALTH_TRAIL_SIGN_PATH; Write-Output $s.Status; if ($s.SignerCertificate) { Write-Output $s.SignerCertificate.Thumbprint }`)
	cmd.Env = append(os.Environ(), "HEALTH_TRAIL_SIGN_PATH="+path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 || !strings.EqualFold(lines[0], "Valid") || len(lines) < 2 {
		if len(lines) > 0 && strings.EqualFold(lines[0], "NotSigned") {
			return "", ErrUnsigned
		}
		return "", ErrBadDownload
	}
	return lines[1], nil
}

// Relaunch starts a helper that waits for this process to exit, deletes
// stage (the old exe) and opens the new exe. The caller quits next.
func Relaunch(exe, stage string, args []string) error {
	var list []string
	for _, a := range args {
		list = append(list, "'"+strings.ReplaceAll(a, "'", "''")+"'")
	}
	script := fmt.Sprintf(`$pidToWait = %d
$stage = '%s'
$exe = '%s'
while (Get-Process -Id $pidToWait -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }
Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
Start-Process -FilePath $exe -ArgumentList @(%s)
`, os.Getpid(), strings.ReplaceAll(stage, "'", "''"), strings.ReplaceAll(exe, "'", "''"), strings.Join(list, ","))
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = detachAttr()
	return cmd.Start()
}
