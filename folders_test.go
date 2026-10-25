package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The folder grants are the window's: over the network port, even from
// this computer or with the token, there is no /api/folders, so nobody can
// pop the open dialog up on the Mac or change what the app may read.
func TestFoldersOnlyInTheWindow(t *testing.T) {
	a := newApp(t.TempDir(), "0")
	defer a.st.Close()
	network := a.listener.handler

	for _, c := range []struct{ name, remote, host string }{
		{"this computer", "127.0.0.1:50000", "127.0.0.1:8721"},
		{"phone with the token", "192.168.1.20:50000", "192.168.1.5:8721"},
	} {
		for _, m := range []string{"GET", "POST", "DELETE"} {
			r := httptest.NewRequest(m, "/api/folders", strings.NewReader(`{"path":"/tmp"}`))
			r.RemoteAddr, r.Host = c.remote, c.host
			r.Header.Set("Authorization", "Bearer "+a.token)
			w := httptest.NewRecorder()
			network.ServeHTTP(w, r)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s over the network: %d %s", c.name, m, w.Code, w.Body)
			}
		}
	}

	// the window has it (GET only: POST would open the dialog)
	w := httptest.NewRecorder()
	a.windowHandler(nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/folders", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"folders":[]`) {
		t.Errorf("window GET /api/folders: %d %s", w.Code, w.Body)
	}
}
