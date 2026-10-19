package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// listener is the app's socket. It can be moved between 127.0.0.1 and all
// interfaces while the app runs, when phone access is switched on or off.
type listener struct {
	handler http.Handler
	mu      sync.Mutex
	srv     *http.Server
	err     error
}

func (l *listener) listen(addr string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		l.srv.Shutdown(ctx)
		cancel()
		l.srv = nil
	}
	ln, err := net.Listen("tcp", addr)
	l.err = err
	if err != nil {
		return err
	}
	l.srv = &http.Server{
		Addr:              addr,
		Handler:           l.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go l.srv.Serve(ln)
	return nil
}

func (l *listener) addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil {
		return ""
	}
	return l.srv.Addr
}

func (l *listener) errText() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		return ""
	}
	return l.err.Error()
}

func (l *listener) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		l.srv.Close()
		l.srv = nil
	}
}

type settings struct {
	path string
	LAN  bool `json:"lan"`
}

func loadSettings(dir string) *settings {
	s := &settings{path: filepath.Join(dir, "settings.json")}
	if b, err := os.ReadFile(s.path); err == nil {
		json.Unmarshal(b, s)
	}
	return s
}

func (s *settings) save() error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(s.path, b, 0o600)
}
