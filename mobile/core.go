// Package mobile exposes the Health Trail local backend for gomobile (iOS).
// It runs the SQLite store, REST API, AI Assistant, local backups, and embedded
// web UI on an ephemeral 127.0.0.1 loopback port behind server.Guard.
package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lee/health-trail/frontend"
	"github.com/lee/health-trail/internal/breaktimer"
	"github.com/lee/health-trail/internal/mcpserver"
	"github.com/lee/health-trail/internal/msg"
	"github.com/lee/health-trail/internal/server"
	"github.com/lee/health-trail/internal/store"
)

// Version is set via -ldflags at build time.
var Version = "1.0.0"

// Core manages the embedded local Health Trail HTTP server and SQLite store.
// All exported methods follow gomobile's supported type subset so they bridge
// cleanly to Swift (MobileCore).
type Core struct {
	mu       sync.Mutex
	dataDir  string
	baseURL  string
	listenAt string
	st       *store.Store
	settings *settings
	httpSrv  *http.Server
	ln       net.Listener
	bgCancel context.CancelFunc
	// uiOverride allows tests to run without requiring frontend/dist.
	uiOverride fs.FS
}

var defaultCore = NewCore()

// NewCore creates a new Core instance.
func NewCore() *Core {
	return &Core{}
}

// Start starts the default Core instance with dataDir and returns its loopback
// base URL (e.g. "http://127.0.0.1:54321").
func Start(dataDir string) (string, error) {
	return defaultCore.Start(dataDir)
}

// Stop stops the default Core instance.
func Stop() error {
	return defaultCore.Stop()
}

// IsRunning reports whether the default Core instance is running.
func IsRunning() bool {
	return defaultCore.IsRunning()
}

// BaseURL returns the loopback base URL of the default Core instance, or "".
func BaseURL() string {
	return defaultCore.BaseURL()
}

// Version returns the build version of HealthTrailCore.
func (c *Core) Version() string {
	return Version
}

// IsRunning reports whether the local server is currently listening.
func (c *Core) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.httpSrv != nil
}

// BaseURL returns the current loopback URL (e.g. "http://127.0.0.1:54321"),
// or an empty string when stopped.
func (c *Core) BaseURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL
}

// Start opens the SQLite store in dataDir and starts the loopback HTTP server.
// Calling Start again with the same dataDir while already running is a no-op
// and returns the existing URL.
func (c *Core) Start(dataDir string) (string, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return "", msg.New("缺少数据目录", "Data directory is required")
	}
	cleanDir := filepath.Clean(dataDir)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.httpSrv != nil {
		if c.dataDir == cleanDir {
			return c.baseURL, nil
		}
		c.stopLocked()
	}

	if err := os.MkdirAll(cleanDir, 0o700); err != nil {
		return "", msg.Errorf("创建数据目录失败：%v", "Couldn't create data directory: %v", err)
	}

	st, err := store.Open(cleanDir)
	if err != nil {
		return "", msg.Errorf("打开数据库失败：%v", "Couldn't open database: %v", err)
	}

	token, err := server.LoadToken(cleanDir)
	if err != nil {
		st.Close()
		return "", msg.Errorf("读取访问令牌失败：%v", "Couldn't load access token: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		st.Close()
		return "", msg.Errorf("启动本地服务失败：%v", "Couldn't start local server: %v", err)
	}

	cfgSettings := loadSettings(cleanDir)
	cfgSettings.LAN = false

	addr := ln.Addr().String()
	baseURL := "http://" + addr

	c.dataDir = cleanDir
	c.listenAt = addr
	c.baseURL = baseURL
	c.st = st
	c.settings = cfgSettings
	c.ln = ln

	uiFS := c.uiOverride
	if uiFS == nil {
		uiFS = frontend.FS()
	}

	mcpSrv := mcpserver.New(st, Version, false)
	mcpHTTP := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpSrv
	}, nil)

	apiSrv := &server.Server{
		Store: st,
		UI:    uiFS,
		MCP:   mcpHTTP,
		Info:  c.info,
		Lang:  cfgSettings.lang,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/mobile/env", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"mobile":   true,
			"platform": "ios",
			"version":  Version,
		})
	})
	mux.HandleFunc("PUT /api/settings", c.putSettings)

	assist := &assistantState{st: st, settings: cfgSettings}
	assist.routes(mux)

	bkp := newBackupManager(st, cleanDir, cfgSettings.lang)
	bkp.routes(mux)

	breaks := &breakTimerState{
		engine:   breaktimer.New(cleanDir, nil),
		settings: cfgSettings,
	}
	breaks.routes(mux)

	bgCtx, bgCancel := context.WithCancel(context.Background())
	c.bgCancel = bgCancel
	go breaks.run(bgCtx)

	mux.Handle("/", apiSrv.Handler())

	guarded := &server.Guard{
		Token: token,
		Next:  mux,
		Lang:  cfgSettings.lang,
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           guarded,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	c.httpSrv = srv

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("mobile server stopped: %v", err)
		}
	}()

	return baseURL, nil
}

// Stop gracefully shuts down the local HTTP server and closes the SQLite store.
func (c *Core) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
	return nil
}

func (c *Core) stopLocked() {
	if c.bgCancel != nil {
		c.bgCancel()
		c.bgCancel = nil
	}
	if c.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.httpSrv.Shutdown(ctx)
		cancel()
		c.httpSrv = nil
	}
	if c.ln != nil {
		_ = c.ln.Close()
		c.ln = nil
	}
	if c.st != nil {
		_ = c.st.Close()
		c.st = nil
	}
	c.baseURL = ""
	c.listenAt = ""
	c.dataDir = ""
	c.settings = nil
}

func (c *Core) info(r *http.Request) map[string]any {
	return map[string]any{
		"version":               Version,
		"mobile":                true,
		"platform":              "ios",
		"lan":                   false,
		"lang":                  c.settings.rawLang(),
		"lists":                 c.settings.lists(),
		"listening":             c.listenAt,
		"listen_error":          "",
		"phone_urls":            nil,
		"mcp_stdio_command":     "",
		"claude_desktop_config": "",
		"claude_code_command":   "",
		"mcp_http_url":          "",
		"mcp_http_config":       "",
		"claude_code_http":      "",
		"codex_command":         "",
		"codex_config":          "",
		"codex_http":            "",
		"mcp_args":              "",
	}
}

func (c *Core) putSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		LAN   *bool                 `json:"lan"`
		Lang  *string               `json:"lang"`
		Lists map[string]listChoice `json:"lists"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, c.settings.msg("无效的请求", "Invalid request"))
		return
	}
	if len(in.Lists) > 0 {
		lists, err := c.settings.mergeLists(in.Lists)
		if err != nil {
			writeError(w, c.settings.errText(err))
			return
		}
		c.settings.mu.Lock()
		c.settings.Lists = lists
		if _, ok := in.Lists["today_metrics"]; ok {
			c.settings.HiddenMetrics = nil
		}
		c.settings.mu.Unlock()
		if err := c.settings.save(); err != nil {
			log.Printf("mobile save settings: %v", err)
		}
	}
	if in.Lang != nil {
		if *in.Lang != "zh" && *in.Lang != "en" {
			writeError(w, c.settings.msg("lang 只能是 zh 或 en", "lang must be zh or en"))
			return
		}
		c.settings.mu.Lock()
		c.settings.Lang = strings.TrimPrefix(*in.Lang, "zh")
		c.settings.mu.Unlock()
		if err := c.settings.save(); err != nil {
			log.Printf("mobile save settings: %v", err)
		}
	}
	if in.LAN != nil && *in.LAN {
		writeError(w, c.settings.msg("手机端不支持局域网共享", "LAN sharing is not available on mobile"))
		return
	}
	writeJSON(w, c.info(r))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
