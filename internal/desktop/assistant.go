package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/lee/health-trail/internal/assistant"
	"github.com/lee/health-trail/internal/server"
)

// assistantRoutes registers 记一笔. speech is true only for the app window:
// `serve` can draft and save from this computer, but it does not turn the
// microphone on. localOnly (the serve handler) answers 404 unless the request
// is from this computer, so a phone that has the token still cannot use it
// or read the key.
func (a *app) assistantRoutes(mux *http.ServeMux, speech bool) {
	a.assistantRoutesOpt(mux, speech, false)
}

func (a *app) serveHandler() http.Handler {
	mux := http.NewServeMux()
	a.assistantRoutesOpt(mux, false, true)
	mux.Handle("/", a.handler())
	return mux
}

func (a *app) assistantRoutesOpt(mux *http.ServeMux, speech, localOnly bool) {
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if localOnly && !server.FromThisComputer(r) {
				http.NotFound(w, r)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /api/assistant", wrap(func(w http.ResponseWriter, r *http.Request) {
		cfg, err := assistant.Load(a.st.Dir)
		if err != nil {
			writeError(w, a.msg("记一笔的配置读不出来", "Couldn't read the capture settings"))
			return
		}
		writeJSON(w, assistantView(cfg, speech && a.ear != nil && a.ear.Available()))
	}))
	mux.HandleFunc("PUT /api/assistant", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			BaseURL  *string `json:"base_url"`
			APIKey   *string `json:"api_key"`
			Model    *string `json:"model"`
			ClearKey bool    `json:"clear_key"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		cfg, err := assistant.Load(a.st.Dir)
		if err != nil {
			writeError(w, a.msg("记一笔的配置读不出来", "Couldn't read the capture settings"))
			return
		}
		cfg, err = cfg.Update(in.BaseURL, in.APIKey, in.Model, in.ClearKey)
		if err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, assistantView(cfg, speech && a.ear != nil && a.ear.Available()))
	}))
	mux.HandleFunc("POST /api/assistant/draft", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		cfg, err := assistant.Load(a.st.Dir)
		if err != nil {
			writeError(w, a.msg("记一笔的配置读不出来", "Couldn't read the capture settings"))
			return
		}
		ctx, stop := context.WithTimeout(r.Context(), 60*time.Second)
		defer stop()
		d, err := assistant.Propose(ctx, a.st, cfg, in.Text, time.Now())
		if err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, d)
	}))
	mux.HandleFunc("POST /api/assistant/commit", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Items []assistant.Item `json:"items"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&in); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		writeJSON(w, assistant.Commit(a.st, in.Items))
	}))
	mux.HandleFunc("POST /api/assistant/speech/start", wrap(func(w http.ResponseWriter, r *http.Request) {
		if !speech {
			writeError(w, a.speechMessage(errSpeechUnavailable))
			return
		}
		if err := a.beginSpeech(); err != nil {
			writeError(w, a.speechMessage(err))
			return
		}
		writeJSON(w, a.speechJSON())
	}))
	mux.HandleFunc("POST /api/assistant/speech/stop", wrap(func(w http.ResponseWriter, r *http.Request) {
		if !speech {
			writeError(w, a.speechMessage(errSpeechUnavailable))
			return
		}
		if _, err := a.endSpeech(); err != nil && !errors.Is(err, errSpeechUnavailable) {
			writeError(w, a.speechMessage(err))
			return
		}
		writeJSON(w, a.speechJSON())
	}))
	mux.HandleFunc("GET /api/assistant/speech", wrap(func(w http.ResponseWriter, r *http.Request) {
		if !speech {
			writeJSON(w, map[string]any{"available": false, "listening": false, "text": ""})
			return
		}
		writeJSON(w, a.speechJSON())
	}))
}

func assistantView(cfg assistant.Config, speech bool) map[string]any {
	return map[string]any{
		"configured": cfg.Configured(),
		"base_url":   cfg.BaseURL,
		"model":      cfg.Model,
		"has_key":    cfg.APIKey != "",
		"speech":     speech,
	}
}

func (a *app) speechJSON() map[string]any {
	available := a.ear != nil && a.ear.Available()
	view := SpeechView{}
	if a.ear != nil {
		view = a.ear.View()
	}
	out := map[string]any{
		"available": available,
		"listening": view.Listening,
		"text":      view.Text,
	}
	if view.ErrCode == 3 {
		out["error"] = a.speechMessage(speechFailure{Code: 3})
	} else if view.ErrCode != 0 && !view.Listening {
		out["error"] = a.speechMessage(speechFailure{Code: view.ErrCode})
	}
	return out
}
