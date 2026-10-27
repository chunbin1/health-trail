package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lee/health-trail/internal/store"
)

const token = "0123456789abcdef0123456789abcdef0123456789abcdef"

func guarded(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	return &Guard{Token: token, Next: s.Handler()}, st
}

func do(h http.Handler, method, url, remote, host string, body string, hdr map[string]string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	r.RemoteAddr = remote
	r.Host = host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGuard(t *testing.T) {
	h, _ := guarded(t)
	local, phone := "127.0.0.1:50000", "192.168.1.20:50000"
	cases := []struct {
		name         string
		remote, host string
		hdr          map[string]string
		url          string
		want         int
	}{
		{"local", local, "127.0.0.1:8721", nil, "/api/overview", 200},
		{"local by name", local, "localhost:8721", nil, "/api/overview", 200},
		{"rebinding", local, "evil.example:8721", nil, "/api/overview", 401},
		{"*.localhost", local, "evil.localhost:8721", nil, "/api/overview", 401},
		{"phone without token", phone, "192.168.1.5:8721", nil, "/api/overview", 401},
		{"phone with bearer", phone, "192.168.1.5:8721", map[string]string{"Authorization": "Bearer " + token}, "/api/overview", 200},
		{"phone wrong token", phone, "192.168.1.5:8721", map[string]string{"Authorization": "Bearer nope"}, "/api/overview", 401},
		{"phone cookie", phone, "192.168.1.5:8721", map[string]string{"Cookie": cookieName + "=" + token}, "/api/overview", 200},
		{"phone ?token on api", phone, "192.168.1.5:8721", nil, "/api/overview?token=" + token, 200},
		{"other site's page", local, "127.0.0.1:8721", map[string]string{"Origin": "https://evil.example"}, "/api/overview", 403},
		{"null origin", local, "127.0.0.1:8721", map[string]string{"Origin": "null"}, "/api/overview", 403},
		{"other local port", local, "127.0.0.1:8721", map[string]string{"Origin": "http://127.0.0.1:3000"}, "/api/overview", 403},
		{"own page", local, "127.0.0.1:8721", map[string]string{"Origin": "http://127.0.0.1:8721"}, "/api/overview", 200},
		{"token doesn't beat a foreign origin", phone, "192.168.1.5:8721", map[string]string{"Origin": "https://evil.example", "Authorization": "Bearer " + token}, "/api/overview", 403},
	}
	for _, c := range cases {
		if w := do(h, "GET", c.url, c.remote, c.host, "", c.hdr); w.Code != c.want {
			t.Errorf("%s: %d, want %d: %s", c.name, w.Code, c.want, w.Body)
		}
	}
	// the token link sets the cookie and drops the token from the address
	w := do(h, "GET", "/?token="+token, phone, "192.168.1.5:8721", "", nil)
	if w.Code != 302 || w.Header().Get("Location") != "/" || !strings.Contains(w.Header().Get("Set-Cookie"), cookieName+"="+token) {
		t.Errorf("token link: %d %v", w.Code, w.Header())
	}
	// a cross-site form post can't write
	w = do(h, "POST", "/api/r/conditions", local, "127.0.0.1:8721", `{"name":"x"}`, map[string]string{"Origin": "https://evil.example"})
	if w.Code != 403 {
		t.Errorf("cross-site post: %d", w.Code)
	}
}

func TestAPI(t *testing.T) {
	h, _ := guarded(t)
	local, host := "127.0.0.1:1", "127.0.0.1:8721"
	w := do(h, "POST", "/api/r/conditions", local, host, `{"name":"急性扁桃体炎","status":"进行中","onset_date":"2026/10/14"}`, nil)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var c map[string]any
	json.Unmarshal(w.Body.Bytes(), &c)
	if c["onset_date"] != "2026-10-14" {
		t.Errorf("created: %v", c)
	}
	if w := do(h, "POST", "/api/r/conditions", local, host, `{"status":"进行中"}`, nil); w.Code != 400 || !strings.Contains(w.Body.String(), "名称") {
		t.Errorf("missing name: %d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", "/api/r/conditions/999", local, host, "", nil); w.Code != 404 {
		t.Errorf("missing id: %d", w.Code)
	}
	if w := do(h, "GET", "/api/r/conditions?status=进行中", local, host, "", nil); !strings.Contains(w.Body.String(), `"total":1`) {
		t.Errorf("filter: %s", w.Body)
	}

	// attachments: stored, served inline when safe, as a download otherwise
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("owner_type", "conditions")
	mw.WriteField("owner_id", "1")
	fw, _ := mw.CreateFormFile("file", "x.html")
	fw.Write([]byte("<script>alert(1)</script>"))
	mw.Close()
	r := httptest.NewRequest("POST", "/api/attachments", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.RemoteAddr, r.Host = local, host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	w = do(h, "GET", "/api/attachments/1", local, host, "", nil)
	if w.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("html attachment served as %v", w.Header())
	}
	// the window sends the bytes as JSON, because a multipart file body
	// does not survive the Mac web view
	jpg := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xd9})
	w = do(h, "POST", "/api/attachments", local, host, `{"owner_type":"conditions","owner_id":1,"filename":"化验.jpg","mime":"image/jpeg","data":"`+jpg+`"}`, nil)
	if w.Code != 200 {
		t.Fatalf("json upload: %d %s", w.Code, w.Body)
	}
	w = do(h, "GET", "/api/attachments/2", local, host, "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("jpeg attachment: %d %v", w.Code, w.Header())
	}
	if w := do(h, "DELETE", "/api/r/conditions/1", local, host, "", nil); w.Code != 200 {
		t.Errorf("delete: %d", w.Code)
	}
	if w := do(h, "GET", "/api/attachments/1", local, host, "", nil); w.Code != 404 {
		t.Errorf("attachment outlived its record: %d", w.Code)
	}
}

// FromThisComputer is the same test Guard uses to let a request in without
// a token; the MCP endpoint uses it to decide who may attach local files.
func TestFromThisComputer(t *testing.T) {
	for _, c := range []struct {
		remote, host string
		want         bool
	}{
		{"127.0.0.1:50000", "127.0.0.1:8721", true},
		{"[::1]:50000", "localhost:8721", true},
		{"127.0.0.1:50000", "evil.example:8721", false},      // DNS rebinding
		{"127.0.0.1:50000", "mac.tail1234.ts.net", false},    // tailscale serve, proxied from the phone
		{"192.168.1.20:50000", "127.0.0.1:8721", false},      // another device
		{"100.101.102.103:50000", "192.168.1.5:8721", false}, // Tailscale
	} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		r.RemoteAddr, r.Host = c.remote, c.host
		if got := FromThisComputer(r); got != c.want {
			t.Errorf("%s → %s: %v, want %v", c.remote, c.host, got, c.want)
		}
	}
}
