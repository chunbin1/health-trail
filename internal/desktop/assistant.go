package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lee/health-trail/internal/assistant"
	"github.com/lee/health-trail/internal/server"
)

// assistantRoutes registers AI助手. speech is true only for the app window:
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
			writeError(w, a.msg("AI助手的配置读不出来", "Couldn't read the AI assistant settings"))
			return
		}
		writeJSON(w, a.assistantView(cfg, speech && a.ear != nil && a.ear.Available()))
	}))
	mux.HandleFunc("PUT /api/assistant", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			BaseURL  *string `json:"base_url"`
			APIKey   *string `json:"api_key"`
			Model    *string `json:"model"`
			ClearKey bool    `json:"clear_key"`
			AutoSave *bool   `json:"auto_save"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		cfg, err := assistant.Load(a.st.Dir)
		if err != nil {
			writeError(w, a.msg("AI助手的配置读不出来", "Couldn't read the AI assistant settings"))
			return
		}
		if in.BaseURL != nil || in.APIKey != nil || in.Model != nil || in.ClearKey {
			cfg, err = cfg.Update(in.BaseURL, in.APIKey, in.Model, in.ClearKey)
			if err != nil {
				writeError(w, err.Error())
				return
			}
		}
		if in.AutoSave != nil {
			if err := a.settings.setAutoSave(*in.AutoSave); err != nil {
				writeError(w, a.msg("没能保存自动保存的开关", "Couldn't save automatic saving"))
				return
			}
		}
		writeJSON(w, a.assistantView(cfg, speech && a.ear != nil && a.ear.Available()))
	}))
	mux.HandleFunc("POST /api/assistant/draft", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID       string           `json:"id"`
			Text     string           `json:"text"`
			Messages []assistant.Turn `json:"messages"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12<<20)).Decode(&in); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		if len(in.Messages) == 0 && strings.TrimSpace(in.Text) != "" {
			in.Messages = []assistant.Turn{{Role: "user", Text: in.Text}}
		}
		job := a.beginDraft(in.ID, in.Messages)
		select {
		case <-r.Context().Done():
			// the window went to the back or the page was left; the draft
			// keeps running and GET /api/assistant/draft returns it
			return
		case <-job.done:
			if job.err != "" {
				writeError(w, job.err)
				return
			}
			writeJSON(w, job.draft)
		}
	}))
	mux.HandleFunc("GET /api/assistant/draft", wrap(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.draftStatus())
	}))
	mux.HandleFunc("GET /api/assistant/chats", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := assistant.ListChats(a.st.Dir)
		if err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, list)
	}))
	mux.HandleFunc("GET /api/assistant/chats/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		c, ok, err := assistant.GetChat(a.st.Dir, r.PathValue("id"))
		if err != nil {
			writeError(w, err.Error())
			return
		}
		if !ok {
			writeError(w, a.msg("没有这段对话", "That chat isn't here"))
			return
		}
		writeJSON(w, c)
	}))
	mux.HandleFunc("DELETE /api/assistant/chats/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		if err := assistant.DeleteChat(a.st.Dir, r.PathValue("id")); err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("PUT /api/assistant/chats", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID       string          `json:"id"`
			Messages json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12<<20)).Decode(&in); err != nil {
			writeError(w, a.msg("无效的请求", "Invalid request"))
			return
		}
		c, err := assistant.SaveChat(a.st.Dir, in.ID, in.Messages)
		if err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, c)
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

// draftRun is one organize call. It outlives the HTTP request: leaving the
// page, or the window going to the back, must not throw the reply away.
type draftRun struct {
	id    string
	text  string
	draft assistant.Draft
	err   string
	done  chan struct{}
	// the tools called so far, shown while the reply is on its way
	mu    sync.Mutex
	steps []assistant.Step
}

func (j *draftRun) stepsSoFar() []assistant.Step {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]assistant.Step{}, j.steps...)
}

func (a *app) beginDraft(id string, turns []assistant.Turn) *draftRun {
	text := ""
	for _, t := range turns {
		if t.Role == "user" || t.Role == "" {
			text = strings.TrimSpace(t.Text)
		}
	}
	if id == "" {
		id = text
	}
	a.draftMu.Lock()
	if a.draft != nil && a.draft.id == id {
		select {
		case <-a.draft.done:
		default:
			job := a.draft
			a.draftMu.Unlock()
			return job
		}
	}
	job := &draftRun{id: id, text: text, done: make(chan struct{})}
	a.draft = job
	a.draftMu.Unlock()
	go func() {
		cfg, err := assistant.Load(a.st.Dir)
		if err == nil {
			// several rounds of reading, each one a request to the model
			ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
			job.draft, err = assistant.Propose(ctx, a.st, cfg, turns, time.Now(), func(s assistant.Step) {
				job.mu.Lock()
				job.steps = append(job.steps, s)
				job.mu.Unlock()
			})
			stop()
		}
		if err != nil {
			job.err = err.Error()
		}
		close(job.done)
	}()
	return job
}

func (a *app) draftStatus() map[string]any {
	a.draftMu.Lock()
	job := a.draft
	a.draftMu.Unlock()
	if job == nil {
		return map[string]any{"state": "none"}
	}
	select {
	case <-job.done:
		if job.err != "" {
			return map[string]any{"state": "error", "id": job.id, "text": job.text, "error": job.err}
		}
		return map[string]any{"state": "done", "id": job.id, "text": job.text, "items": job.draft.Items, "unclear": job.draft.Unclear, "reply": job.draft.Reply, "steps": job.draft.Steps, "usage": job.draft.Usage}
	default:
		return map[string]any{"state": "pending", "id": job.id, "text": job.text, "steps": job.stepsSoFar()}
	}
}

func (a *app) assistantView(cfg assistant.Config, speech bool) map[string]any {
	return map[string]any{
		"configured": cfg.Configured(),
		"base_url":   cfg.BaseURL,
		"model":      cfg.Model,
		"has_key":    cfg.APIKey != "",
		"speech":     speech,
		"auto_save":  a.settings.autoSave(),
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
