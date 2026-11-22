package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lee/health-trail/internal/breaktimer"
)

func (a *app) breakTimerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/break-timer", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.breaks.Snapshot())
	})
	mux.HandleFunc("PUT /api/break-timer/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg breaktimer.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		if err := a.breaks.UpdateConfig(cfg); err != nil {
			writeError(w, a.errText(err))
			return
		}
		writeJSON(w, a.breaks.Snapshot())
	})
	mux.HandleFunc("POST /api/break-timer/action", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		var (
			snap breaktimer.Snapshot
			err  error
		)
		switch in.Action {
		case "start":
			snap, err = a.breaks.Start()
		case "pause":
			snap = a.breaks.Pause()
		case "resume":
			snap, err = a.breaks.Resume()
		case "stop":
			snap, err = a.breaks.Stop()
		case "break_now":
			snap, err = a.breaks.StartBreakNow()
		case "complete":
			snap, err = a.breaks.CompleteBreak(true)
		case "complete_idle":
			snap, err = a.breaks.CompleteBreak(false)
		case "snooze":
			snap, err = a.breaks.Snooze()
		case "skip":
			snap, err = a.breaks.SkipBreak()
		case "water_add":
			snap, err = a.breaks.AddWater(1)
		case "water_sub":
			snap, err = a.breaks.AddWater(-1)
		default:
			writeError(w, a.msg("无效的操作", "Invalid action"))
			return
		}
		if err != nil {
			writeError(w, a.errText(err))
			return
		}
		writeJSON(w, snap)
	})
	mux.HandleFunc("POST /api/break-timer/hide", func(w http.ResponseWriter, r *http.Request) {
		if bw := a.getBreakWin(); bw != nil {
			bw.Hide()
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}

func (a *app) setBreakWindows(win, breakWin *application.WebviewWindow) {
	a.breakWinMu.Lock()
	a.mainWin = win
	a.breakWin = breakWin
	a.breakWinMu.Unlock()
}

func (a *app) getBreakWin() *application.WebviewWindow {
	a.breakWinMu.RLock()
	defer a.breakWinMu.RUnlock()
	return a.breakWin
}

func (a *app) getMainWin() *application.WebviewWindow {
	a.breakWinMu.RLock()
	defer a.breakWinMu.RUnlock()
	return a.mainWin
}

func (a *app) bindBreakEvents() {
	a.breaks.SetOnChange(func(ev breaktimer.Event) {
		if a.shell != nil {
			a.shell.setBreakTimer(ev.Snapshot)
		}
		bw := a.getBreakWin()
		mw := a.getMainWin()
		if bw != nil {
			if ev.OpenBreak {
				bw.Center()
				bw.Show()
				bw.Focus()
			} else if ev.HideBreak {
				bw.Hide()
			}
		}
		if ev.NotifyUI {
			if mw != nil {
				mw.ExecJS(`window.dispatchEvent(new Event("break-timer-changed"))`)
			}
			if bw != nil {
				bw.ExecJS(`window.dispatchEvent(new Event("break-timer-changed"))`)
			}
		}
		if ev.PlaySound {
			if bw != nil && (ev.OpenBreak || bw.IsVisible()) {
				bw.ExecJS(`window.dispatchEvent(new Event("break-timer-chime"))`)
			} else if mw != nil {
				mw.ExecJS(`window.dispatchEvent(new Event("break-timer-chime"))`)
			}
		}
	})
	if a.shell != nil {
		a.shell.setBreakTimer(a.breaks.Snapshot())
	}
}

func (a *app) runBreakTimer(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			a.breaks.Tick()
		}
	}
}
