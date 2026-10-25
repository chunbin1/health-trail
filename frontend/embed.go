// Package frontend is the built web UI (dist, made by `make ui`), embedded
// in the binary. A Go file here because go:embed can't reach up a folder.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS is the built UI, its index.html at the top.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
