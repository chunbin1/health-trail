package mobile

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lee/health-trail/internal/assistant"
	"github.com/lee/health-trail/internal/store"
)

type draftRun struct {
	id    string
	text  string
	draft assistant.Draft
	err   string
	done  chan struct{}

	mu    sync.Mutex
	steps []assistant.Step
}

func (j *draftRun) stepsSoFar() []assistant.Step {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]assistant.Step{}, j.steps...)
}

type assistantState struct {
	st       *store.Store
	settings *settings

	mu    sync.Mutex
	draft *draftRun
}

func (a *assistantState) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/assistant", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := assistant.Load(a.st.Dir)
		if err != nil {
			writeError(w, a.settings.msg("AI助手的配置读不出来", "Couldn't read the AI assistant settings"))
			return
		}
		writeJSON(w, a.view(cfg))
	})
	mux.HandleFunc("PUT /api/assistant", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			BaseURL  *string `json:"base_url"`
			APIKey   *string `json:"api_key"`
			Model    *string `json:"model"`
			ClearKey bool    `json:"clear_key"`
			AutoSave *bool   `json:"auto_save"`
			RawLog   *bool   `json:"raw_log"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			writeError(w, a.settings.msg("无效的请求", "Invalid request"))
			return
		}
		cfg, err := assistant.Load(a.st.Dir)
		if err != nil {
			writeError(w, a.settings.msg("AI助手的配置读不出来", "Couldn't read the AI assistant settings"))
			return
		}
		if in.BaseURL != nil || in.APIKey != nil || in.Model != nil || in.ClearKey {
			cfg, err = cfg.Update(in.BaseURL, in.APIKey, in.Model, in.ClearKey)
			if err != nil {
				writeError(w, a.settings.errText(err))
				return
			}
		}
		if in.RawLog != nil {
			cfg, err = cfg.SetRawLog(*in.RawLog)
			if err != nil {
				writeError(w, a.settings.errText(err))
				return
			}
		}
		if in.AutoSave != nil {
			if err := a.settings.setAutoSave(*in.AutoSave); err != nil {
				writeError(w, a.settings.msg("没能保存自动保存的开关", "Couldn't save automatic saving"))
				return
			}
		}
		writeJSON(w, a.view(cfg))
	})
	mux.HandleFunc("POST /api/assistant/check", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := assistant.Load(a.st.Dir)
		if err != nil {
			writeError(w, a.settings.msg("AI助手的配置读不出来", "Couldn't read the AI assistant settings"))
			return
		}
		ctx, stop := context.WithTimeout(r.Context(), 60*time.Second)
		defer stop()
		if err := assistant.Check(ctx, cfg); err != nil {
			writeError(w, a.settings.errText(err))
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/assistant/draft", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID       string           `json:"id"`
			Text     string           `json:"text"`
			Messages []assistant.Turn `json:"messages"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12<<20)).Decode(&in); err != nil {
			writeError(w, a.settings.msg("无效的请求", "Invalid request"))
			return
		}
		if len(in.Messages) == 0 && strings.TrimSpace(in.Text) != "" {
			in.Messages = []assistant.Turn{{Role: "user", Text: in.Text}}
		}
		job := a.beginDraft(in.ID, in.Messages)
		select {
		case <-r.Context().Done():
			return
		case <-job.done:
			if job.err != "" {
				writeError(w, job.err)
				return
			}
			writeJSON(w, job.draft)
		}
	})
	mux.HandleFunc("GET /api/assistant/draft", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.draftStatus())
	})
	mux.HandleFunc("GET /api/assistant/log", func(w http.ResponseWriter, r *http.Request) {
		view, err := assistant.ReadLog(a.st.Dir)
		if err != nil {
			writeError(w, a.settings.msg("AI助手的日志读不出来", "Couldn't read the AI assistant's log"))
			return
		}
		writeJSON(w, view)
	})
	mux.HandleFunc("DELETE /api/assistant/log", func(w http.ResponseWriter, r *http.Request) {
		if err := assistant.ClearLog(a.st.Dir); err != nil {
			writeError(w, a.settings.msg("AI助手的日志没能清空", "Couldn't clear the AI assistant's log"))
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/assistant/chats", func(w http.ResponseWriter, r *http.Request) {
		list, err := assistant.ListChats(a.st.Dir)
		if err != nil {
			writeError(w, a.settings.errText(err))
			return
		}
		writeJSON(w, list)
	})
	mux.HandleFunc("GET /api/assistant/chats/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok, err := assistant.GetChat(a.st.Dir, r.PathValue("id"))
		if err != nil {
			writeError(w, a.settings.errText(err))
			return
		}
		if !ok {
			writeError(w, a.settings.msg("没有这段对话", "That chat isn't here"))
			return
		}
		writeJSON(w, c)
	})
	mux.HandleFunc("DELETE /api/assistant/chats/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := assistant.DeleteChat(a.st.Dir, r.PathValue("id")); err != nil {
			writeError(w, a.settings.errText(err))
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("PUT /api/assistant/chats", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID       string          `json:"id"`
			Messages json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12<<20)).Decode(&in); err != nil {
			writeError(w, a.settings.msg("无效的请求", "Invalid request"))
			return
		}
		c, err := assistant.SaveChat(a.st.Dir, in.ID, in.Messages)
		if err != nil {
			writeError(w, a.settings.errText(err))
			return
		}
		writeJSON(w, c)
	})
	mux.HandleFunc("POST /api/assistant/commit", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Items []assistant.Item `json:"items"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&in); err != nil {
			writeError(w, a.settings.msg("无效的请求", "Invalid request"))
			return
		}
		writeJSON(w, assistant.Commit(a.st, in.Items, a.settings.lang()))
	})
	// On iOS, users can dictate directly via the native keyboard microphone.
	mux.HandleFunc("GET /api/assistant/speech", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"available": false, "listening": false, "text": ""})
	})
	mux.HandleFunc("POST /api/assistant/speech/start", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, a.settings.msg("请使用系统键盘上的语音听写", "Use the system keyboard dictation"))
	})
	mux.HandleFunc("POST /api/assistant/speech/stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"available": false, "listening": false, "text": ""})
	})
}

func (a *assistantState) beginDraft(id string, turns []assistant.Turn) *draftRun {
	text := ""
	for _, t := range turns {
		if t.Role == "user" || t.Role == "" {
			text = strings.TrimSpace(t.Text)
		}
	}
	if id == "" {
		id = text
	}
	a.mu.Lock()
	if a.draft != nil && a.draft.id == id {
		select {
		case <-a.draft.done:
		default:
			job := a.draft
			a.mu.Unlock()
			return job
		}
	}
	job := &draftRun{id: id, text: text, done: make(chan struct{})}
	a.draft = job
	a.mu.Unlock()

	go func() {
		cfg, err := assistant.Load(a.st.Dir)
		if err == nil {
			ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
			cfg.Lang = a.settings.lang()
			job.draft, err = assistant.Propose(ctx, a.st, cfg, turns, time.Now(), func(s assistant.Step) {
				job.mu.Lock()
				job.steps = append(job.steps, s)
				job.mu.Unlock()
			})
			stop()
		}
		if err != nil {
			job.err = a.settings.errText(err)
		}
		close(job.done)
	}()
	return job
}

func (a *assistantState) draftStatus() map[string]any {
	a.mu.Lock()
	job := a.draft
	a.mu.Unlock()
	if job == nil {
		return map[string]any{"state": "none"}
	}
	select {
	case <-job.done:
		if job.err != "" {
			return map[string]any{"state": "error", "id": job.id, "text": job.text, "error": job.err}
		}
		return map[string]any{
			"state":   "done",
			"id":      job.id,
			"text":    job.text,
			"items":   job.draft.Items,
			"unclear": job.draft.Unclear,
			"reply":   job.draft.Reply,
			"steps":   job.draft.Steps,
			"usage":   job.draft.Usage,
		}
	default:
		return map[string]any{"state": "pending", "id": job.id, "text": job.text, "steps": job.stepsSoFar()}
	}
}

func (a *assistantState) view(cfg assistant.Config) map[string]any {
	return map[string]any{
		"configured": cfg.Configured(),
		"base_url":   cfg.BaseURL,
		"model":      cfg.Model,
		"has_key":    cfg.APIKey != "",
		"speech":     false,
		"auto_save":  a.settings.autoSave(),
		"raw_log":    cfg.RawLog,
		"providers":  assistant.Providers,
	}
}
