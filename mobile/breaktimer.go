package mobile

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/lee/health-trail/internal/breaktimer"
)

type breakTimerState struct {
	engine   *breaktimer.Engine
	settings *settings
}

func (b *breakTimerState) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/break-timer", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, b.engine.Snapshot())
	})
	mux.HandleFunc("PUT /api/break-timer/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg breaktimer.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, b.settings.msg("无效的请求", "Invalid request"))
			return
		}
		if err := b.engine.UpdateConfig(cfg); err != nil {
			writeError(w, b.settings.errText(err))
			return
		}
		writeJSON(w, b.engine.Snapshot())
	})
	mux.HandleFunc("POST /api/break-timer/action", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, b.settings.msg("无效的请求", "Invalid request"))
			return
		}
		var (
			snap breaktimer.Snapshot
			err  error
		)
		switch in.Action {
		case "start":
			snap, err = b.engine.Start()
		case "pause":
			snap = b.engine.Pause()
		case "resume":
			snap, err = b.engine.Resume()
		case "stop":
			snap, err = b.engine.Stop()
		case "break_now":
			snap, err = b.engine.StartBreakNow()
		case "complete":
			snap, err = b.engine.CompleteBreak(true)
		case "complete_idle":
			snap, err = b.engine.CompleteBreak(false)
		case "snooze":
			snap, err = b.engine.Snooze()
		case "skip":
			snap, err = b.engine.SkipBreak()
		case "water_add":
			snap, err = b.engine.AddWater(1)
		case "water_sub":
			snap, err = b.engine.AddWater(-1)
		default:
			writeError(w, b.settings.msg("无效的操作", "Invalid action"))
			return
		}
		if err != nil {
			writeError(w, b.settings.errText(err))
			return
		}
		writeJSON(w, snap)
	})
	mux.HandleFunc("POST /api/break-timer/hide", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	})
}

func (b *breakTimerState) run(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			b.engine.Tick()
		}
	}
}
