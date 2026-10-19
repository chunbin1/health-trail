package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Guard decides who may reach the API, the MCP endpoint and the UI over the
// network. Health records are the most private data a person has, so:
//
//   - A request from this computer is let in only when its Host is a
//     loopback name. A web page on some other site that rebinds its own
//     domain to 127.0.0.1 arrives with that domain as Host, and is refused.
//     *.localhost names are refused too: WebKit resolves them through DNS.
//   - A request from another device (the phone, over Wi-Fi or Tailscale)
//     needs the access token: as a Bearer header, as the cookie set when
//     the token link was opened, or as ?token= on that link.
//   - A browser request whose Origin isn't this server's own is refused,
//     whoever sends it, so no other page can post into the records.
type Guard struct {
	Token string
	Next  http.Handler
}

const cookieName = "hv_token"

func (g *Guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")

	if o := r.Header.Get("Origin"); o != "" && !sameOrigin(o, r.Host) {
		writeErr(w, http.StatusForbidden, "拒绝跨站请求")
		return
	}
	if t := r.URL.Query().Get("token"); t != "" && g.valid(t) {
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Value: t, Path: "/", HttpOnly: true,
			SameSite: http.SameSiteStrictMode, MaxAge: 365 * 24 * 3600,
		})
		if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/mcp" {
			// drop the token from the address bar and history
			u := *r.URL
			q := u.Query()
			q.Del("token")
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.RequestURI(), http.StatusFound)
			return
		}
		g.Next.ServeHTTP(w, r)
		return
	}
	if g.valid(bearer(r)) {
		g.Next.ServeHTTP(w, r)
		return
	}
	if c, err := r.Cookie(cookieName); err == nil && g.valid(c.Value) {
		g.Next.ServeHTTP(w, r)
		return
	}
	if fromLoopback(r) && loopbackHost(r.Host) {
		g.Next.ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" {
		writeErr(w, http.StatusUnauthorized, "需要访问令牌：请在电脑上的「连接」页面复制手机访问链接")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>健康档案</title>
<body style="font:16px -apple-system,sans-serif;padding:2em;max-width:30em;margin:auto">
<h2>需要访问令牌</h2><p>请在电脑上打开健康档案 →「连接」页面，用手机扫码或打开里面的访问链接。</p></body>`))
}

func (g *Guard) valid(t string) bool {
	return g.Token != "" && t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(g.Token)) == 1
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func fromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" { // includes "null"
		return false
	}
	return strings.EqualFold(u.Host, host)
}

// LoadToken reads the access token from dir, making one the first time.
func LoadToken(dir string) (string, error) {
	p := filepath.Join(dir, "token")
	b, err := os.ReadFile(p)
	if err == nil && len(strings.TrimSpace(string(b))) >= 32 {
		return strings.TrimSpace(string(b)), nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf)
	return t, os.WriteFile(p, []byte(t+"\n"), 0o600)
}

// LANAddrs lists this computer's private IPv4 addresses (Wi-Fi, Ethernet,
// Tailscale), the ones a phone can reach it on.
func LANAddrs() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip := ipn.IP.To4()
			// RFC 1918, plus Tailscale's 100.64/10
			if ip.IsPrivate() || (ip[0] == 100 && ip[1]&0xC0 == 64) {
				out = append(out, ip.String())
			}
		}
	}
	return out
}
