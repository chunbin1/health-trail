package desktop

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/lee/health-trail/internal/breaktimer"
	"github.com/lee/health-trail/internal/update"
)

//go:embed tray.png
var trayIcon []byte

// shell keeps the app running in the menu bar so reminders and backups go
// on with the window closed: closing the window hides it (and the Dock
// icon), the menu bar icon opens it again or quits, and the app can open at
// login without a window.
type shell struct {
	wapp          *application.App
	win           *application.WebviewWindow
	lang          func() string
	onHide        func() // the window was closed
	breaks        *breaktimer.Engine
	menu          *application.Menu
	breakStatus   *application.MenuItem
	breakToggle   *application.MenuItem
	breakAction   *application.MenuItem
	breakIdle     *application.MenuItem
	breakSnooze   *application.MenuItem
	breakWater    *application.MenuItem
	open          *application.MenuItem
	update        *application.MenuItem // a newer version, shown when one is found
	newer         string
	atLogin       *application.MenuItem
	quitItem      *application.MenuItem
	tray          *application.SystemTray
	built         string // the language, newer version and break menu state the Windows tray menu was built with
	breakSnap     breaktimer.Snapshot
	hasBreakSnap  bool
	lastTrayLabel string
	lastTooltip   string
	lastMenuKey   string
	// refresh rebuilds the menu; one at a time, as the start, a language
	// switch and an update check can each ask for it at once
	mu sync.Mutex
}

func newShell(wapp *application.App, win *application.WebviewWindow, lang func() string, onHide func(), breaks *breaktimer.Engine) *shell {
	s := &shell{wapp: wapp, win: win, lang: lang, onHide: onHide, breaks: breaks}
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		s.hide()
	})
	s.menu = wapp.NewMenu()
	if breaks != nil {
		s.breakStatus = s.menu.Add("")
		s.breakStatus.SetEnabled(false)
		s.breakToggle = s.menu.Add("").OnClick(func(*application.Context) {
			go s.toggleBreakTimer()
		})
		s.breakAction = s.menu.Add("").OnClick(func(*application.Context) {
			go s.triggerBreakAction()
		})
		s.breakIdle = s.menu.Add("").OnClick(func(*application.Context) {
			go func() {
				_, _ = s.breaks.CompleteBreak(false)
			}()
		})
		s.breakSnooze = s.menu.Add("").OnClick(func(*application.Context) {
			go func() {
				_, _ = s.breaks.Snooze()
			}()
		})
		s.breakWater = s.menu.Add("").OnClick(func(*application.Context) {
			go func() {
				_, _ = s.breaks.AddWater(1)
			}()
		})
		s.menu.AddSeparator()
		s.breakSnap = breaks.Snapshot()
		s.hasBreakSnap = true
	}
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
		// a click opens the window, as Windows tray icons do; the menu is
		// on the right button
		tray.OnClick(s.show)
		// whether it opens at login is read again each time the menu opens:
		// it can be switched off in Windows Settings meanwhile
		tray.OnRightClick(func() {
			go func() {
				s.refresh()
				tray.OpenMenu()
			}()
		})
	} else {
		tray.SetTemplateIcon(trayIcon)
	}
	tray.SetMenu(s.menu)
	s.tray = tray
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

func (s *shell) toggleBreakTimer() {
	if s.breaks == nil {
		return
	}
	switch s.breaks.Snapshot().State {
	case breaktimer.StateWorking, breaktimer.StateBreak:
		s.breaks.Pause()
	case breaktimer.StatePaused:
		_, _ = s.breaks.Resume()
	default:
		_, _ = s.breaks.Start()
	}
}

func (s *shell) triggerBreakAction() {
	if s.breaks == nil {
		return
	}
	switch s.breaks.Snapshot().State {
	case breaktimer.StateBreak, breaktimer.StateBreakDone:
		_, _ = s.breaks.CompleteBreak(true)
	default:
		_, _ = s.breaks.StartBreakNow()
	}
}

// setBreakTimer updates the menu bar countdown (macOS) or tray tooltip (Windows)
// and refreshes the tray menu when the state, counts, snooze minutes, or
// minute bucket changes.
func (s *shell) setBreakTimer(snap breaktimer.Snapshot) {
	s.mu.Lock()
	s.breakSnap = snap
	s.hasBreakSnap = true
	en := s.lang() == "en"
	s.updateTrayStatusLocked(snap, en)
	key := breakMenuKey(snap, en)
	needRefresh := key != s.lastMenuKey
	s.mu.Unlock()
	if needRefresh {
		s.refresh()
	}
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
	var menuKey string
	if s.hasBreakSnap && s.breakStatus != nil {
		snap := s.breakSnap
		s.updateTrayStatusLocked(snap, en)
		s.applyBreakMenuLocked(snap, en)
		menuKey = breakMenuKey(snap, en)
		s.lastMenuKey = menuKey
	}
	s.open.SetLabel(pick("打开健康足迹", "Open Health Trail"))
	s.update.SetLabel(pick("有新版本 "+s.newer+"…", "Update to "+s.newer+"…"))
	s.update.SetHidden(s.newer == "")
	s.atLogin.SetLabel(pick("开机自动启动", "Open at Login"))
	s.atLogin.SetChecked(s.autostart())
	s.quitItem.SetLabel(pick("退出健康足迹", "Quit Health Trail"))
	if runtime.GOOS != "windows" {
		s.menu.Update()
		return
	}
	// On Windows Update builds a menu of its own, not the one the tray
	// shows, and leaves the items changing that one. The tray's menu takes
	// labels and checks in place, but items that hide/show or change on
	// the tray only appear by building the tray menu again.
	if built := s.lang() + "|" + s.newer + "|" + menuKey; built != s.built {
		s.built = built
		s.tray.SetMenu(s.menu)
	}
}

func (s *shell) updateTrayStatusLocked(snap breaktimer.Snapshot, en bool) {
	if s.tray == nil {
		return
	}
	if runtime.GOOS != "windows" {
		lbl := trayLabel(snap, en)
		if lbl != "" {
			ensureTrayMonospacedFont()
		}
		if lbl != s.lastTrayLabel {
			s.lastTrayLabel = lbl
			s.tray.SetLabel(lbl)
		}
		return
	}
	tip := trayTooltip(snap, en)
	if tip != s.lastTooltip {
		s.lastTooltip = tip
		s.tray.SetTooltip(tip)
	}
}

func (s *shell) applyBreakMenuLocked(snap breaktimer.Snapshot, en bool) {
	pick := func(zh, e string) string {
		if en {
			return e
		}
		return zh
	}
	mins := (snap.RemainingSec + 59) / 60
	w, g, b := snap.Today.Water, snap.Config.WaterGoal, snap.Today.Breaks
	var status string
	switch snap.State {
	case breaktimer.StateWorking:
		status = fmt.Sprintf(pick("距离休息 %d 分钟 · 喝水 %d/%d · 休息 %d", "Break in %dm · Water %d/%d · Breaks %d"), mins, w, g, b)
	case breaktimer.StateBreak:
		status = fmt.Sprintf(pick("正在休息 %d 分钟 · 喝水 %d/%d", "On break %dm · Water %d/%d"), mins, w, g)
	case breaktimer.StateBreakDone:
		status = fmt.Sprintf(pick("休息完成 · 喝水 %d/%d · 休息 %d", "Break done · Water %d/%d · Breaks %d"), w, g, b)
	case breaktimer.StatePaused:
		status = fmt.Sprintf(pick("已暂停（余 %d 分钟）· 喝水 %d/%d", "Paused (%dm left) · Water %d/%d"), mins, w, g)
	case breaktimer.StateOutsideHours:
		status = fmt.Sprintf(pick("非工作时段 · 喝水 %d/%d · 休息 %d", "Outside hours · Water %d/%d · Breaks %d"), w, g, b)
	default:
		status = fmt.Sprintf(pick("未开启计时 · 喝水 %d/%d · 休息 %d", "Timer off · Water %d/%d · Breaks %d"), w, g, b)
	}
	s.breakStatus.SetLabel(status)

	switch snap.State {
	case breaktimer.StateWorking, breaktimer.StateBreak:
		s.breakToggle.SetLabel(pick("暂停计时", "Pause timer"))
		s.breakToggle.SetHidden(false)
	case breaktimer.StatePaused:
		s.breakToggle.SetLabel(pick("继续计时", "Resume timer"))
		s.breakToggle.SetHidden(false)
	case breaktimer.StateBreakDone:
		s.breakToggle.SetHidden(true)
	default:
		s.breakToggle.SetLabel(pick("开始专注", "Start focus"))
		s.breakToggle.SetHidden(false)
	}

	switch snap.State {
	case breaktimer.StateBreak:
		s.breakAction.SetLabel(pick("休息完了", "Finish break"))
	case breaktimer.StateBreakDone:
		s.breakAction.SetLabel(pick("开始下一轮", "Start next round"))
	default:
		s.breakAction.SetLabel(pick("现在休息", "Take a break now"))
	}

	s.breakIdle.SetLabel(pick("暂不开始", "Not now"))
	s.breakIdle.SetHidden(snap.State != breaktimer.StateBreakDone)

	hideSnooze := snap.State == breaktimer.StateIdle || snap.State == breaktimer.StateOutsideHours || snap.State == breaktimer.StateBreakDone
	s.breakSnooze.SetHidden(hideSnooze)
	s.breakSnooze.SetLabel(fmt.Sprintf(pick("推迟 %d 分钟", "Snooze %d min"), snap.Config.SnoozeMinutes))
	s.breakWater.SetLabel(pick("记一杯水 (+1)", "Log 1 cup (+1)"))
}

func breakMenuKey(snap breaktimer.Snapshot, en bool) string {
	lang := "zh"
	if en {
		lang = "en"
	}
	mins := (snap.RemainingSec + 59) / 60
	return fmt.Sprintf("%s|%d|%d|%d|%d|%d|%s", snap.State, snap.Today.Water, snap.Config.WaterGoal, snap.Today.Breaks, snap.Config.SnoozeMinutes, mins, lang)
}

func trayLabel(snap breaktimer.Snapshot, en bool) string {
	if !snap.Config.MenuBarCountdown {
		return ""
	}
	switch snap.State {
	case breaktimer.StateWorking:
		return " " + formatMMSS(snap.RemainingSec)
	case breaktimer.StateBreak:
		if en {
			return " Break " + formatMMSS(snap.RemainingSec)
		}
		return " 休息 " + formatMMSS(snap.RemainingSec)
	case breaktimer.StateBreakDone:
		if en {
			return " Break done"
		}
		return " 休息完成"
	case breaktimer.StatePaused:
		if en {
			return " Paused"
		}
		return " 已暂停"
	default:
		return ""
	}
}

func trayTooltip(snap breaktimer.Snapshot, en bool) string {
	pick := func(zh, e string) string {
		if en {
			return e
		}
		return zh
	}
	if !snap.Config.MenuBarCountdown {
		return pick("健康足迹", "Health Trail")
	}
	switch snap.State {
	case breaktimer.StateWorking:
		return fmt.Sprintf(pick("距离休息 %s · 喝水 %d/%d", "Break in %s · Water %d/%d"), formatMMSS(snap.RemainingSec), snap.Today.Water, snap.Config.WaterGoal)
	case breaktimer.StateBreak:
		return fmt.Sprintf(pick("正在休息 %s · 喝水 %d/%d", "On break %s · Water %d/%d"), formatMMSS(snap.RemainingSec), snap.Today.Water, snap.Config.WaterGoal)
	case breaktimer.StateBreakDone:
		return fmt.Sprintf(pick("休息完成 · 喝水 %d/%d", "Break done · Water %d/%d"), snap.Today.Water, snap.Config.WaterGoal)
	case breaktimer.StatePaused:
		return fmt.Sprintf(pick("已暂停 %s · 喝水 %d/%d", "Paused %s · Water %d/%d"), formatMMSS(snap.RemainingSec), snap.Today.Water, snap.Config.WaterGoal)
	default:
		return pick("健康足迹", "Health Trail")
	}
}

func formatMMSS(sec int) string {
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%02d:%02d", sec/60, sec%60)
}
