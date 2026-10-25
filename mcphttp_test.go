package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lee/health-trail/internal/server"
	"github.com/lee/health-trail/internal/store"
)

// Over HTTP, through Guard as the app serves it: a client on this computer
// is offered add_attachment, the phone with the token is not.
func TestMCPOverHTTPAttachOnlyHere(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef"
	h := &server.Guard{Token: token, Next: (&server.Server{Store: st, MCP: mcpHandler(st)}).Handler()}

	tools := func(remote, host string, hdr map[string]string) (int, []string) {
		t.Helper()
		session := ""
		post := func(body string) (int, string) {
			r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
			r.RemoteAddr, r.Host = remote, host
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			if session != "" {
				r.Header.Set("Mcp-Session-Id", session)
			}
			for k, v := range hdr {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if s := w.Header().Get("Mcp-Session-Id"); s != "" {
				session = s
			}
			return w.Code, w.Body.String()
		}
		code, body := post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
		if code != http.StatusOK {
			return code, nil
		}
		post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		code, body = post(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
		var names []string
		sc := bufio.NewScanner(strings.NewReader(body))
		sc.Buffer(make([]byte, 1<<20), 1<<22)
		for sc.Scan() {
			line := strings.TrimPrefix(sc.Text(), "data: ")
			var msg struct {
				Result struct {
					Tools []struct{ Name string } `json:"tools"`
				} `json:"result"`
			}
			if json.Unmarshal([]byte(line), &msg) == nil {
				for _, tl := range msg.Result.Tools {
					names = append(names, tl.Name)
				}
			}
		}
		return code, names
	}
	has := func(names []string) bool {
		for _, n := range names {
			if n == "add_attachment" {
				return true
			}
		}
		return false
	}

	code, here := tools("127.0.0.1:50000", "127.0.0.1:8721", nil)
	if code != 200 || len(here) != 15 || !has(here) {
		t.Errorf("this computer: %d %v", code, here)
	}
	code, phone := tools("192.168.1.20:50000", "192.168.1.5:8721", map[string]string{"Authorization": "Bearer " + token})
	if code != 200 || len(phone) != 14 || has(phone) {
		t.Errorf("phone with token: %d %v", code, phone)
	}
	if code, _ := tools("192.168.1.20:50000", "192.168.1.5:8721", nil); code != http.StatusUnauthorized {
		t.Errorf("phone without token: %d", code)
	}
}
