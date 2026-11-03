// Command winupdate packs and signs the Windows update zip.
//
//	go run ./scripts/winupdate pack -version 1.2.0 -exe health-trail.exe -out dist
//	    writes dist/HealthTrail-1.2.0-windows.zip (the exe and version.txt).
//	    With UPDATE_SIGNING_KEY set (or -sign, which requires it), also
//	    writes the .sig next to it and checks it against the public key
//	    built into the app, so a release with the wrong key fails here.
//	go run ./scripts/winupdate keygen -out <file>
//	    makes a new key: the private key goes to <file>, the public key is
//	    printed for internal/update/sign.go.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lee/health-trail/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "pack":
		fl := flag.NewFlagSet("pack", flag.ExitOnError)
		version := fl.String("version", "", "the release's version, without v")
		exe := fl.String("exe", "health-trail.exe", "the built exe")
		out := fl.String("out", "dist", "folder for the zip")
		sign := fl.Bool("sign", false, "fail without UPDATE_SIGNING_KEY")
		fl.Parse(os.Args[2:])
		if *version == "" {
			fatal("-version is required")
		}
		if err := pack(*version, *exe, *out, *sign); err != nil {
			fatal(err.Error())
		}
	case "keygen":
		fl := flag.NewFlagSet("keygen", flag.ExitOnError)
		out := fl.String("out", "", "file for the private key (must not exist)")
		fl.Parse(os.Args[2:])
		if *out == "" {
			fatal("-out is required")
		}
		if err := keygen(*out); err != nil {
			fatal(err.Error())
		}
	default:
		usage()
	}
}

func pack(version, exe, out string, mustSign bool) error {
	body, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		data []byte
	}{{"health-trail.exe", body}, {"version.txt", []byte(version + "\n")}} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		if _, err := w.Write(f.data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	name := filepath.Join(out, "HealthTrail-"+version+"-windows.zip")
	if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Println(name)

	seed := strings.TrimSpace(os.Getenv("UPDATE_SIGNING_KEY"))
	if seed == "" {
		if mustSign {
			return fmt.Errorf("UPDATE_SIGNING_KEY is not set")
		}
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(seed)
	if err != nil || len(raw) != ed25519.SeedSize {
		return fmt.Errorf("UPDATE_SIGNING_KEY isn't a base64 Ed25519 seed")
	}
	sig := update.Sign(ed25519.NewKeyFromSeed(raw), buf.Bytes())
	if err := update.CheckSignature(buf.Bytes(), sig); err != nil {
		return fmt.Errorf("UPDATE_SIGNING_KEY doesn't match the public key in internal/update/sign.go: %v", err)
	}
	if err := os.WriteFile(name+update.SigSuffix, sig, 0o644); err != nil {
		return err
	}
	fmt.Println(name + update.SigSuffix)
	return nil
}

func keygen(out string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, base64.StdEncoding.EncodeToString(priv.Seed())); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

func usage() {
	fatal("usage: winupdate pack -version V [-exe health-trail.exe] [-out dist] [-sign] | keygen -out FILE")
}

func fatal(s string) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(1)
}
