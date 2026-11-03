package server

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// On Windows a type can come from the registry. Even with .svg and .js
// set to something else there, the UI's logo is an image and its script
// runs: the page gets the types the files really have.
func TestUITypesIgnoreTheSystem(t *testing.T) {
	// what a Windows registry entry does to Go's list
	mime.AddExtensionType(".svg", "text/xml")
	mime.AddExtensionType(".js", "text/plain")
	t.Cleanup(func() {
		mime.AddExtensionType(".svg", "image/svg+xml")
		mime.AddExtensionType(".js", "text/javascript; charset=utf-8")
	})
	ui := fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html>")},
		"icon.svg":         {Data: []byte("<svg/>")},
		"assets/index.js":  {Data: []byte("1")},
		"assets/index.css": {Data: []byte("a{}")},
	}
	h := spa(ui)
	for path, want := range map[string]string{
		"/icon.svg":         "image/svg+xml",
		"/assets/index.js":  "text/javascript; charset=utf-8",
		"/assets/index.css": "text/css; charset=utf-8",
		"/":                 "text/html; charset=utf-8",
		"/settings":         "text/html; charset=utf-8", // a client route
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %q, want %q", path, rec.Code, rec.Header().Get("Content-Type"), want)
		}
	}
}
