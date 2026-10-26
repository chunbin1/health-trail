// Package desktop is the app around the data: the Mac window and menu bar
// icon, the local service (REST API, web UI, MCP over HTTP) behind
// server.Guard, MCP over stdio, backups, reminders and the Calendar sync.
// main picks which of these runs.
package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/lee/health-trail/frontend"
	"github.com/lee/health-trail/internal/attachopen"
	"github.com/lee/health-trail/internal/calsync"
	"github.com/lee/health-trail/internal/folders"
	"github.com/lee/health-trail/internal/mcpserver"
	"github.com/lee/health-trail/internal/reminders"
	"github.com/lee/health-trail/internal/server"
	"github.com/lee/health-trail/internal/store"
	"github.com/lee/health-trail/internal/update"
)

// Version is the app's Version, which main sets from the build.
var Version = "0.1.0"

// DefaultPort is the local service's port.
const DefaultPort = "8721"

// DefaultDataDir is where the data lives unless -data says otherwise.
func DefaultDataDir() string {
	// HEALTH_VAULT_DATA is the name from before the app was called Health Trail
	for _, k := range []string{"HEALTH_TRAIL_DATA", "HEALTH_VAULT_DATA"} {
		if d := os.Getenv(k); d != "" {
			return d
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	// still the name from before the app was called Health Trail: the data
	// moves once, into the App Store sandbox, rather than twice
	return filepath.Join(base, "HealthVault")
}

func openStore(dir string) *store.Store {
	st, err := store.Open(dir)
	if err != nil {
		log.Fatalf("打开数据库失败（%s）：%v", dir, err)
	}
	return st
}

// RunMCP speaks MCP on stdin/stdout. Stdout belongs to the protocol, so
// nothing else may print there: the log goes to stderr.
func RunMCP(dataDir string) {
	log.SetOutput(os.Stderr)
	st := openStore(dataDir)
	defer st.Close()
	// the folders granted in the window, so add_attachment can read there
	// in the App Store sandbox too
	folders.New(dataDir).Open()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mcpserver.New(st, Version, true).Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

type app struct {
	st       *store.Store
	srv      *server.Server
	listener *listener
	settings *settings
	backups  *backups
	updates  *updates
	grants   *folders.Grants    // folders the person let the sandboxed app read
	shell    *shell             // the window and menu bar icon, in the app only
	remind   *reminders.Service // in the app only
	cal      *calsync.Sync      // in the app only
	token    string
	port     string
	openDir  string // copies of attachments opened from the window, in the app only
}

func newApp(dataDir, port string) *app {
	st := openStore(dataDir)
	token, err := server.LoadToken(dataDir)
	if err != nil {
		log.Fatalf("读取访问令牌失败：%v", err)
	}
	a := &app{st: st, token: token, port: port, settings: loadSettings(dataDir), backups: newBackups(st, dataDir), grants: folders.New(dataDir)}
	a.updates = newUpdates(a.settings)
	a.grants.Open()
	// phone access is hidden until it's reworked: a choice saved earlier no
	// longer opens the app to the network (serve -lan still does, on purpose)
	a.settings.LAN = false
	a.srv = &server.Server{
		Store: st,
		UI:    frontend.FS(),
		MCP:   mcpHandler(st),
		Info:  a.info,
	}
	a.listener = &listener{handler: &server.Guard{Token: token, Next: a.handler()}}
	return a
}

// mcpHandler serves MCP over HTTP. add_attachment reads files on this
// computer, so only a client on this computer gets it; the phone (with the
// token) gets the server without it. "On this computer" is what the socket
// says, so a tunnel that ends here (an SSH port forward) counts as this
// computer: whoever can open one already has the Mac.
func mcpHandler(st *store.Store) http.Handler {
	local, remote := mcpserver.New(st, Version, true), mcpserver.New(st, Version, false)
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if server.FromThisComputer(r) {
			return local
		}
		return remote
	}, nil)
}

// handler adds the settings route, which needs the listener, to the API.
func (a *app) handler() http.Handler {
	api := a.srv.Handler()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			LAN   *bool                 `json:"lan"`
			Lang  *string               `json:"lang"`
			Lists map[string]listChoice `json:"lists"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, `{"error":"无效的请求"}`, http.StatusBadRequest)
			return
		}
		if len(in.Lists) > 0 {
			lists, err := a.settings.mergeLists(in.Lists)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			a.settings.mu.Lock()
			a.settings.Lists = lists
			if _, ok := in.Lists["today_metrics"]; ok {
				a.settings.HiddenMetrics = nil // carried into the list
			}
			a.settings.mu.Unlock()
			if err := a.settings.save(); err != nil {
				log.Printf("保存设置失败：%v", err)
			}
		}
		if in.Lang != nil {
			if *in.Lang != "zh" && *in.Lang != "en" {
				http.Error(w, `{"error":"lang 只能是 zh 或 en"}`, http.StatusBadRequest)
				return
			}
			a.settings.mu.Lock()
			a.settings.Lang = strings.TrimPrefix(*in.Lang, "zh")
			a.settings.mu.Unlock()
			if err := a.settings.save(); err != nil {
				log.Printf("保存设置失败：%v", err)
			}
			if a.shell != nil {
				a.shell.refresh() // the menu bar menu follows the language
			}
		}
		if in.LAN != nil && *in.LAN {
			http.Error(w, `{"error":"手机访问暂时关闭"}`, http.StatusBadRequest)
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
	a.backups.routes(mux)
	mux.Handle("/", api)
	return mux
}

// windowHandler is what the app's own window loads: the same API, marked as
// coming from the window, plus a folder dialog that only the window may open
// (over the network it would pop up on the Mac for anyone with the token).
func (a *app) windowHandler(wapp func() *application.App) http.Handler {
	h := a.handler()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/backup/choose-dir", func(w http.ResponseWriter, r *http.Request) {
		title := "选择备份文件夹"
		if a.settings.lang() == "en" {
			title = "Choose a backup folder"
		}
		dir, err := wapp().Dialog.OpenFile().SetTitle(title).
			CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
			PromptForSingleSelection()
		if err != nil {
			// macOS's message can name a folder; it goes to the log only
			log.Printf("选择备份文件夹失败：%v", err)
			writeError(w, a.msg("没能打开选择文件夹的窗口，请再试一次", "Couldn't open the folder picker; try again"))
			return
		}
		writeJSON(w, map[string]string{"dir": dir}) // "" when cancelled
	})
	a.folderRoutes(mux, wapp)
	a.updates.routes(mux)
	// the release's page in the browser, for its notes and the dmg; only a
	// page on GitHub, whatever the release says
	mux.HandleFunc("POST /api/update/page", func(w http.ResponseWriter, r *http.Request) {
		page := update.ReleasesPage
		if rel := a.updates.latest(); rel != nil && strings.HasPrefix(rel.Page, "https://github.com/") {
			page = rel.Page
		}
		wapp().Browser.OpenURL(page)
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/attachments/{id}/open", a.openAttachmentHandler)
	mux.HandleFunc("DELETE /api/attachments/{id}", a.deleteAttachmentHandler(h))
	// whether macOS shows the reminders, and a way to its settings if not
	mux.HandleFunc("GET /api/notifications", func(w http.ResponseWriter, r *http.Request) {
		out := map[string]any{"allowed": nil}
		if a.remind != nil {
			if allowed, known := a.remind.Allowed(); known {
				out["allowed"] = allowed
			}
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("POST /api/notifications/settings", func(w http.ResponseWriter, r *http.Request) {
		wapp().Browser.OpenURL("x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=" + bundleID)
		writeJSON(w, map[string]bool{"ok": true})
	})
	// writing appointments to Calendar and medicines to Reminders
	mux.HandleFunc("GET /api/calsync", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.calStatus())
	})
	mux.HandleFunc("PUT /api/calsync", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Events    *bool `json:"events"`
			Reminders *bool `json:"reminders"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || a.cal == nil {
			writeError(w, "无效的请求")
			return
		}
		var err error
		if in.Events != nil {
			err = a.cal.Set(false, *in.Events)
		}
		if in.Reminders != nil && err == nil {
			err = a.cal.Set(true, *in.Reminders)
		}
		if err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, a.calStatus())
	})
	mux.HandleFunc("POST /api/calsync/privacy", func(w http.ResponseWriter, r *http.Request) {
		pane := "Privacy_Calendars"
		if r.URL.Query().Get("reminders") == "1" {
			pane = "Privacy_Reminders"
		}
		wapp().Browser.OpenURL("x-apple.systempreferences:com.apple.preference.security?" + pane)
		writeJSON(w, map[string]bool{"ok": true})
	})
	// opening at login is the window's to change, like the folder dialog
	mux.HandleFunc("GET /api/autostart", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"enabled": a.shell != nil && a.shell.autostart()})
	})
	mux.HandleFunc("PUT /api/autostart", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || a.shell == nil {
			writeError(w, "无效的请求")
			return
		}
		if err := a.shell.setAutostart(in.Enabled); err != nil {
			log.Printf("设置开机启动失败：%v", err)
			writeError(w, a.msg("没能更改开机启动，可以在「系统设置 → 通用 → 登录项」里改", "Couldn't change opening at login; you can change it in System Settings → General → Login Items"))
			return
		}
		writeJSON(w, map[string]bool{"enabled": a.shell.autostart()})
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), inWindow{}, true)))
	}))
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
	if a.st.Dir != DefaultDataDir() {
		args = append(args, "-data", a.st.Dir)
	}
	desktop := map[string]any{"mcpServers": map[string]any{"health-trail": map[string]any{"command": exe, "args": args}}}
	desktopJSON, _ := json.MarshalIndent(desktop, "", "  ")
	httpURL := "http://127.0.0.1:" + a.port + "/mcp"
	// Codex keeps MCP servers in ~/.codex/config.toml, shared by its CLI and
	// editor extension; a TOML basic string escapes like a JSON one
	tomlArgs := make([]string, len(args))
	for i, s := range args {
		tomlArgs[i] = tomlString(s)
	}
	codexConfig := fmt.Sprintf("[mcp_servers.health-trail]\ncommand = %s\nargs = [%s]", tomlString(exe), strings.Join(tomlArgs, ", "))
	var phone []string
	if a.settings.LAN {
		for _, ip := range server.LANAddrs() {
			phone = append(phone, fmt.Sprintf("http://%s:%s/?token=%s", ip, a.port, a.token))
		}
	}
	return map[string]any{
		"version":               Version,
		"lan":                   a.settings.LAN,
		"lang":                  a.settings.lang(),
		"lists":                 a.settings.lists(),
		"listening":             a.listener.addr(),
		"listen_error":          a.listener.errText(),
		"phone_urls":            phone,
		"mcp_stdio_command":     exe,
		"claude_desktop_config": string(desktopJSON),
		"claude_code_command":   "claude mcp add health-trail -s user -- " + shellJoin(exe, args),
		"mcp_http_url":          httpURL,
		"claude_code_http":      "claude mcp add --transport http health-trail -s user " + httpURL,
		"codex_command":         "codex mcp add health-trail -- " + shellJoin(exe, args),
		"codex_config":          codexConfig,
		"codex_http":            "codex mcp add health-trail --url " + httpURL,
		"mcp_args":              strings.Join(args, " "),
	}
}

// shellJoin writes a command line to paste into a terminal: the program
// quoted, and any argument with a space (a data folder under "Application
// Support") quoted too.
func shellJoin(exe string, args []string) string {
	parts := []string{fmt.Sprintf("%q", exe)}
	for _, s := range args {
		if strings.ContainsAny(s, " '\"") {
			s = fmt.Sprintf("%q", s)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func tomlString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func RunServe(dataDir, port string, lanFlag bool) {
	a := newApp(dataDir, port)
	defer a.st.Close()
	if lanFlag {
		a.settings.LAN = true
	}
	if err := a.listener.listen(a.addr()); err != nil {
		log.Fatal(err)
	}
	log.Printf("健康足迹已启动：http://127.0.0.1:%s  数据目录：%s", port, dataDir)
	if a.settings.LAN {
		for _, u := range a.info(nil)["phone_urls"].([]string) {
			log.Printf("手机访问：%s", u)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go a.backups.loop(ctx)
	<-ctx.Done()
	a.listener.close()
	a.backups.quit()
}

func RunApp(dataDir, port string) {
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
	var wapp *application.App
	ctx, stopBackups := context.WithCancel(context.Background())
	go a.backups.loop(ctx)
	// a test copy on other data, or a test build with a bundle id of its
	// own (make appstore-test), writes to calendars of its own
	calTitle := "健康足迹"
	if id := runningBundleID(); dataDir != DefaultDataDir() || id != "" && id != bundleID {
		calTitle = "健康足迹（测试）"
	}
	a.openDir = attachopen.Dir(os.TempDir(), instanceID(dataDir))
	attachopen.Clear(a.openDir) // what a crash or a force quit left behind
	a.cal = calsync.New(a.st, &calsync.EventKit{}, dataDir, calTitle)
	go a.cal.Loop(ctx)
	a.remind = reminders.New(a.st, notifications.New(), a.settings.lang, a.cal.Syncing)
	wapp = application.New(application.Options{
		Name:        "健康足迹",
		Description: "个人健康档案",
		Assets:      application.AssetOptions{Handler: a.windowHandler(func() *application.App { return wapp })},
		OnShutdown:  func() { stopBackups(); a.backups.quit(); attachopen.Clear(a.openDir) },
		Services:    services(a.remind, dataDir),
		// closing the window leaves the app in the menu bar (see shell)
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		// one window per data directory: a second launch on the same data
		// shows the open (or hidden) window, while a test copy on other data
		// runs apart
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: instanceID(dataDir),
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
				if a.shell != nil {
					a.openLink(win, linkIn(d.Args))
				}
			},
		},
	})
	win = wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "健康足迹",
		Width:            1240,
		Height:           820,
		MinWidth:         380,
		MinHeight:        500,
		URL:              "/?shell=mac",
		Hidden:           true, // shown once started, unless opened at login
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarHiddenInset,
			Backdrop: application.MacBackdropTranslucent,
		},
	})
	a.shell = newShell(wapp, win, a.settings.lang, a.backups.windowClosed)
	a.updates.backup = a.backups.beforeUpdate
	a.updates.quit = wapp.Quit
	a.updates.changed = func(u *updates) { a.shell.setUpdate(u.latest()) }
	go a.updates.loop(ctx)
	wapp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		// opened at login: reminders and backups run, the window waits for a click
		if launchedAtLogin() {
			setInDock(false)
		} else {
			a.shell.show()
		}
		a.shell.refresh()
	})
	// a link from Calendar or Reminders, to the running app or one it starts
	wapp.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		a.openLink(win, e.Context().URL())
	})
	if err := wapp.Run(); err != nil {
		log.Fatal(err)
	}
}

// services are the app's background services: the reminders, except for a
// test copy on other data. Notifications belong to the bundle id, which a
// test copy shares with the real app: its start would clear the real app's
// scheduled reminders and put test ones on the person's Mac.
func services(r *reminders.Service, dataDir string) []application.Service {
	if dataDir != DefaultDataDir() {
		log.Printf("测试数据目录：不发系统通知")
		return nil
	}
	return []application.Service{application.NewService(r)}
}

// calStatus is what the settings page shows of the Calendar and Reminders
// sync (none outside the app).
func (a *app) calStatus() map[string]any {
	if a.cal == nil {
		return map[string]any{}
	}
	return a.cal.Status()
}

// bundleID is the app's identifier (build/Info.plist).
const bundleID = "com.chunbin1.healthtrail"

var bundleIDRe = regexp.MustCompile(`<key>CFBundleIdentifier</key>\s*<string>([^<]+)</string>`)

// runningBundleID is the bundle id of the .app this binary runs from, or ""
// outside one.
func runningBundleID() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "..", "Info.plist"))
	if err != nil {
		return ""
	}
	if m := bundleIDRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

func instanceID(dataDir string) string {
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	sum := sha256.Sum256([]byte(dataDir))
	return bundleID + "." + hex.EncodeToString(sum[:6])
}

func RunInfo(dataDir, port string) {
	a := newApp(dataDir, port)
	defer a.st.Close()
	i := a.info(nil)
	fmt.Printf("数据目录：%s\n\nClaude Desktop（claude_desktop_config.json）：\n%s\n\nClaude Code：\n  %s\n\nCodex：\n  %s\n\n", dataDir, i["claude_desktop_config"], i["claude_code_command"], i["codex_command"])
}
