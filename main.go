// health-vault: a personal health record that lives on your own computer,
// with a Mac window, a web UI a phone can open, and MCP tools for Claude.
//
//	health-vault             the desktop app (window + local server)
//	health-vault serve       the server only (no window), e.g. on a mini PC
//	health-vault mcp         MCP over stdio, for Claude Desktop / Claude Code
//	health-vault info        where the data is, how to connect
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lee/health-vault/internal/mcpserver"
	"github.com/lee/health-vault/internal/server"
	"github.com/lee/health-vault/internal/store"
)

//go:embed all:frontend/dist
var embedded embed.FS

var version = "0.1.0"

const defaultPort = "8721"

func defaultDataDir() string {
	if d := os.Getenv("HEALTH_VAULT_DATA"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "HealthVault")
}

func main() {
	cmd, args := "app", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	// macOS passes -psn_… to apps opened from Finder on older systems
	if len(args) > 0 && strings.HasPrefix(args[0], "-psn_") {
		args = nil
	}
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	dataDir := fl.String("data", defaultDataDir(), "数据目录（也可用环境变量 HEALTH_VAULT_DATA）")
	port := fl.String("port", defaultPort, "本地服务端口")
	lan := fl.Bool("lan", false, "serve：允许局域网/Tailscale 中的设备（手机）访问，需令牌")
	fl.Parse(args)

	switch cmd {
	case "mcp":
		runMCP(*dataDir)
	case "serve":
		runServe(*dataDir, *port, *lan)
	case "app":
		runApp(*dataDir, *port)
	case "info":
		runInfo(*dataDir, *port)
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "用法：health-vault [app|serve|mcp|info|version] [-data 目录] [-port 端口] [-lan]\n")
		os.Exit(2)
	}
}

func openStore(dir string) *store.Store {
	st, err := store.Open(dir)
	if err != nil {
		log.Fatalf("打开数据库失败（%s）：%v", dir, err)
	}
	return st
}

// runMCP speaks MCP on stdin/stdout. Stdout belongs to the protocol, so
// nothing else may print there: the log goes to stderr.
func runMCP(dataDir string) {
	log.SetOutput(os.Stderr)
	st := openStore(dataDir)
	defer st.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mcpserver.New(st, version).Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

func uiFS() fs.FS {
	sub, err := fs.Sub(embedded, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	return sub
}

type app struct {
	st       *store.Store
	srv      *server.Server
	listener *listener
	settings *settings
	token    string
	port     string
}

func newApp(dataDir, port string) *app {
	st := openStore(dataDir)
	token, err := server.LoadToken(dataDir)
	if err != nil {
		log.Fatalf("读取访问令牌失败：%v", err)
	}
	a := &app{st: st, token: token, port: port, settings: loadSettings(dataDir)}
	mcpSrv := mcpserver.New(st, version)
	a.srv = &server.Server{
		Store: st,
		UI:    uiFS(),
		MCP:   mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpSrv }, nil),
		Info:  a.info,
	}
	a.listener = &listener{handler: &server.Guard{Token: token, Next: a.handler()}}
	return a
}

// handler adds the settings route, which needs the listener, to the API.
func (a *app) handler() http.Handler {
	api := a.srv.Handler()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			LAN *bool `json:"lan"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, `{"error":"无效的请求"}`, http.StatusBadRequest)
			return
		}
		if in.LAN != nil && *in.LAN != a.settings.LAN {
			a.settings.LAN = *in.LAN
			if err := a.settings.save(); err != nil {
				log.Printf("保存设置失败：%v", err)
			}
			if err := a.listener.listen(a.addr()); err != nil {
				log.Printf("重新监听失败：%v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(a.info(r))
	})
	mux.Handle("/", api)
	return mux
}

func (a *app) addr() string {
	if a.settings.LAN {
		return "0.0.0.0:" + a.port
	}
	return "127.0.0.1:" + a.port
}

// info is what the 连接 page shows: how Claude and the phone connect.
// Only callers who already got past the guard (this computer, or a device
// holding the token) can read it, so it may carry the token.
func (a *app) info(r *http.Request) map[string]any {
	exe, _ := os.Executable()
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	args := []string{"mcp"}
	if a.st.Dir != defaultDataDir() {
		args = append(args, "-data", a.st.Dir)
	}
	desktop := map[string]any{"mcpServers": map[string]any{"health-vault": map[string]any{"command": exe, "args": args}}}
	desktopJSON, _ := json.MarshalIndent(desktop, "", "  ")
	httpURL := "http://127.0.0.1:" + a.port + "/mcp"
	var phone []string
	if a.settings.LAN {
		for _, ip := range server.LANAddrs() {
			phone = append(phone, fmt.Sprintf("http://%s:%s/?token=%s", ip, a.port, a.token))
		}
	}
	return map[string]any{
		"version":               version,
		"lan":                   a.settings.LAN,
		"listening":             a.listener.addr(),
		"listen_error":          a.listener.errText(),
		"phone_urls":            phone,
		"mcp_stdio_command":     exe,
		"claude_desktop_config": string(desktopJSON),
		"claude_code_command":   fmt.Sprintf("claude mcp add health-vault -s user -- %q %s", exe, strings.Join(args, " ")),
		"mcp_http_url":          httpURL,
		"claude_code_http":      "claude mcp add --transport http health-vault -s user " + httpURL,
	}
}

func runServe(dataDir, port string, lanFlag bool) {
	a := newApp(dataDir, port)
	defer a.st.Close()
	if lanFlag {
		a.settings.LAN = true
	}
	if err := a.listener.listen(a.addr()); err != nil {
		log.Fatal(err)
	}
	log.Printf("健康档案已启动：http://127.0.0.1:%s  数据目录：%s", port, dataDir)
	if a.settings.LAN {
		for _, u := range a.info(nil)["phone_urls"].([]string) {
			log.Printf("手机访问：%s", u)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	a.listener.close()
}

func runApp(dataDir, port string) {
	a := newApp(dataDir, port)
	defer a.st.Close()
	// The window works without the socket (it talks to the handler in
	// process); the socket is for Claude over HTTP and for the phone. If the
	// port is taken — `serve` already running — the window still opens.
	if err := a.listener.listen(a.addr()); err != nil {
		log.Printf("本地服务未启动：%v", err)
	}
	defer a.listener.close()

	var win *application.WebviewWindow
	wapp := application.New(application.Options{
		Name:        "健康档案",
		Description: "个人健康档案",
		Assets:      application.AssetOptions{Handler: a.handler()},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		// one window per data directory: a second launch on the same data
		// focuses the open window, while a test copy on other data runs apart
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: instanceID(dataDir),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if win != nil {
					win.Restore()
					win.Focus()
				}
			},
		},
	})
	win = wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "健康档案",
		Width:            1240,
		Height:           820,
		MinWidth:         380,
		MinHeight:        500,
		URL:              "/?shell=mac",
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarHiddenInset,
			Backdrop: application.MacBackdropTranslucent,
		},
	})
	if err := wapp.Run(); err != nil {
		log.Fatal(err)
	}
}

func instanceID(dataDir string) string {
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	sum := sha256.Sum256([]byte(dataDir))
	return "com.health-vault.app." + hex.EncodeToString(sum[:6])
}

func runInfo(dataDir, port string) {
	a := newApp(dataDir, port)
	defer a.st.Close()
	i := a.info(nil)
	fmt.Printf("数据目录：%s\n\nClaude Desktop（claude_desktop_config.json）：\n%s\n\nClaude Code：\n  %s\n\n", dataDir, i["claude_desktop_config"], i["claude_code_command"])
}
