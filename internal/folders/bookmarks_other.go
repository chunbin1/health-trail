//go:build !darwin

package folders

import "errors"

// Outside the Mac App Sandbox a folder the person picked stays readable, so
// the bookmark is the path itself.
func makeBookmark(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("empty path")
	}
	return []byte(path), nil
}

func openBookmark(b []byte) (path string, stale bool, err error) {
	if len(b) == 0 {
		return "", false, errors.New("empty bookmark")
	}
	return string(b), false, nil
}
