package desktop

import (
	_ "embed"
	"log"

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
	tray.SetTemplateIcon(trayIcon)
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

func (s *shell) autostart() bool {
	on, err := s.wapp.Autostart.IsEnabled()
	return err == nil && on
}

func (s *shell) setAutostart(on bool) error {
	var err error
	if on {
		err = s.wapp.Autostart.Enable()
	} else {
		err = s.wapp.Autostart.Disable()
	}
	s.refresh()
	return err
}

// setUpdate shows the newer version found in the menu, or hides it (nil).
func (s *shell) setUpdate(r *update.Release) {
	s.newer = ""
	if r != nil {
		s.newer = r.Version
	}
	s.refresh()
}

// refresh puts the menu in the UI's language and shows whether the app
// opens at login (which can also change in System Settings).
func (s *shell) refresh() {
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
