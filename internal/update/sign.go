package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Windows has no Developer ID: the update zip is signed with our own
// Ed25519 key instead (release.yml, scripts/winupdate). The private key is
// the UPDATE_SIGNING_KEY secret; a copy is kept offline. To replace it, add
// the new public key here and ship a release signed with the old one before
// signing with the new one.
var publicKeys = []string{
	"48OvIDgR/TkA4nOcPqyIzSzLz2ZRNNBSVNjnkcLdJm0=",
}

// Channel is "release" in a copy release.yml built (-X …update.Channel=release).
// Only those replace themselves; a local or CI build is updated by hand.
var Channel = ""

// SigSuffix names the signature next to the zip it signs.
const SigSuffix = ".sig"

// Sign is the signature file for data: base64 and a newline.
func Sign(key ed25519.PrivateKey, data []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, data)) + "\n")
}

// CheckSignature accepts data only with a signature file made by one of
// our keys.
func CheckSignature(data, sig []byte) error {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature isn't one", ErrBadDownload)
	}
	for _, k := range publicKeys {
		pub, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return errors.New("a built-in public key is broken")
		}
		if ed25519.Verify(pub, data, raw) {
			return nil
		}
	}
	return fmt.Errorf("%w: signature", ErrBadDownload)
}
