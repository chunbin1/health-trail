package desktop

import (
	_ "embed"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/lee/health-trail/internal/update"
)

//go:embed tray.png
var trayIcon []byte

// shell keeps the app running in the menu bar so reminders and backups go
// on with the window closed: closing the window hides it (and the Dock
// icon), the menu bar icon opens it again or quits, and the app can open at
// login without a window.
type shell struct {
	wapp     *application.App
	win      *application.WebviewWindow
	lang     func() string
	onHide   func() // the window was closed
	menu     *application.Menu
	open     *application.MenuItem
	update   *application.MenuItem // a newer version, shown when one is found
	newer    string
	atLogin  *application.MenuItem
	quitItem *application.MenuItem
	// refresh rebuilds the menu; one at a time, as the start, a language
	// switch and an update check can each ask for it at once
	mu sync.Mutex
}

func newShell(wapp *application.App, win *application.WebviewWindow, lang func() string, onHide func()) *shell {
	s := &shell{wapp: wapp, win: win, lang: lang, onHide: onHide}
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		s.hide()
	})
	s.menu = wapp.NewMenu()
	s.open = s.menu.Add("").OnClick(func(*application.Context) { s.show() })
	s.update = s.menu.Add("").OnClick(func(*application.Context) {
		s.show()
		s.win.ExecJS(`location.hash = "/settings"`)
	})
	s.update.SetHidden(true)
	s.menu.AddSeparator()
	s.atLogin = s.menu.AddCheckbox("", false).OnClick(func(ctx *application.Context) {
		if err := s.setAutostart(ctx.IsChecked()); err != nil {
			log.Printf("开机自动启动设置失败：%v", err)
		}
	})
	s.menu.AddSeparator()
	s.quitItem = s.menu.Add("").OnClick(func(*application.Context) { wapp.Quit() })
	tray := wapp.SystemTray.New()
	if runtime.GOOS == "windows" {
		// Windows draws a tray icon as it is: the black menu-bar template
		// would vanish on the dark taskbar, so it gets the colored app icon
		tray.SetIcon(appIcon)
	} else {
		tray.SetTemplateIcon(trayIcon)
	}
	tray.SetMenu(s.menu)
	s.refresh()
	return s
}

func (s *shell) show() {
	setInDock(true)
	s.win.Show()
	s.win.Focus()
}

func (s *shell) hide() {
	s.win.Hide()
	setInDock(false)
	if s.onHide != nil {
		go s.onHide()
	}
}

// autostart says whether the app opens at login: registered, and on Windows
// not turned off since in Settings → Apps → Startup.
func (s *shell) autostart() bool {
	on, err := s.wapp.Autostart.IsEnabled()
	return err == nil && on && !startupDisabled(s.autostartName())
}

// autostartName is the Windows login entry registered for this copy: the
// value's name under Run (an older one may still have Wails's default
// name), "" when there is none.
func (s *shell) autostartName() string {
	st, err := s.wapp.Autostart.Status()
	if err != nil || !st.Enabled {
		return ""
	}
	return st.Path[strings.LastIndexAny(st.Path, `\/`)+1:]
}

func (s *shell) setAutostart(on bool) error {
	old := s.autostartName()
	var err error
	if on {
		err = s.wapp.Autostart.EnableWithOptions(autostartOptions())
		if err == nil {
			// on in the app is on: an "off" from Windows Settings, under
			// this name or the one the entry had before, no longer holds
			clearStartupDisabled(autostartID)
			clearStartupDisabled(old)
		}
	} else {
		err = s.wapp.Autostart.Disable()
	}
	s.refresh()
	return err
}

// autostartID names the Windows login entry. Without it Wails derives one
// from the app's name, which for 健康足迹 is the generic "wails-app".
const autostartID = "HealthTrail"

// autostartOptions: on Windows the login start carries -login, the only way
// the app can tell it was started at login and keep the window closed.
func autostartOptions() application.AutostartOptions {
	if runtime.GOOS == "windows" {
		return application.AutostartOptions{Identifier: autostartID, Arguments: []string{"-login"}}
	}
	return application.AutostartOptions{}
}

// renewAutostart writes the Windows login entry again, so one made before
// it carried -login, or under the old name, starts this copy right; one left
// at the place the exe was moved from points here again. An entry turned off
// in Windows Settings stays off.
func (s *shell) renewAutostart() {
	if runtime.GOOS != "windows" {
		return
	}
	if !s.autostart() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		if p, err := filepath.EvalSymlinks(exe); err == nil {
			exe = p
		}
		if s.autostartName() != "" || !movedAutostart(exe) {
			return
		}
		log.Printf("开机自动启动指向的旧位置已经没有 App，改成 %s", exe)
	}
	if err := s.wapp.Autostart.EnableWithOptions(autostartOptions()); err != nil {
		log.Printf("更新开机自动启动失败：%v", err)
	}
}

// setUpdate shows the newer version found in the menu, or hides it (nil).
func (s *shell) setUpdate(r *update.Release) {
	s.mu.Lock()
	s.newer = ""
	if r != nil {
		s.newer = r.Version
	}
	s.mu.Unlock()
	s.refresh()
}

// refresh puts the menu in the UI's language and shows whether the app
// opens at login (which can also change in System Settings).
func (s *shell) refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	en := s.lang() == "en"
	pick := func(zh, e string) string {
		if en {
			return e
		}
		return zh
	}
	s.open.SetLabel(pick("打开健康足迹", "Open Health Trail"))
	s.update.SetLabel(pick("有新版本 "+s.newer+"…", "Update to "+s.newer+"…"))
	s.update.SetHidden(s.newer == "")
	s.atLogin.SetLabel(pick("开机自动启动", "Open at Login"))
	s.atLogin.SetChecked(s.autostart())
	s.quitItem.SetLabel(pick("退出健康足迹", "Quit Health Trail"))
	s.menu.Update()
}
