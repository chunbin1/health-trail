package desktop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAssistantNotOnThePhone(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, "0")
	defer a.st.Close()

	// the Mac app's API, even from this computer, has no capture routes
	for _, path := range []string{"/api/assistant", "/api/assistant/draft", "/api/assistant/commit", "/api/assistant/speech/start"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = "127.0.0.1:9"
		r.Host = "127.0.0.1:8721"
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("handler %s: %d %s", path, w.Code, w.Body)
		}
		r = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"text":"hi"}`))
		r.RemoteAddr = "192.168.1.20:9"
		r.Host = "192.168.1.5:8721"
		r.Header.Set("Authorization", "Bearer "+a.token)
		w = httptest.NewRecorder()
		a.listener.handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("phone %s: %d %s", path, w.Code, w.Body)
		}
	}

	// the window has it, and the reply does not contain the key
	w := httptest.NewRecorder()
	a.windowHandler(nil).ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/assistant",
		strings.NewReader(`{"base_url":"https://api.example.com/v1","api_key":"sk-test-secret","model":"m"}`)))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "sk-test-secret") || !strings.Contains(w.Body.String(), `"configured":true`) || !strings.Contains(w.Body.String(), `"auto_save":true`) {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	raw, _ := os.ReadFile(dir + "/assistant.json")
	if !strings.Contains(string(raw), "sk-test-secret") {
		t.Fatalf("file %s", raw)
	}
	w = httptest.NewRecorder()
	a.windowHandler(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/assistant", nil))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "sk-test-secret") || !strings.Contains(w.Body.String(), `"auto_save":true`) {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	a.windowHandler(nil).ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/assistant", strings.NewReader(`{"auto_save":false}`)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"auto_save":false`) {
		t.Fatalf("auto off: %d %s", w.Code, w.Body)
	}

	// serve answers this computer and refuses a phone
	local := a.serveHandler()
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/assistant", nil)
	req.RemoteAddr = "127.0.0.1:9"
	req.Host = "127.0.0.1:8722"
	local.ServeHTTP(w, req)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "sk-test-secret") || strings.Contains(w.Body.String(), `"speech":true`) {
		t.Fatalf("serve: %d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/assistant", nil)
	req.RemoteAddr = "192.168.1.20:9"
	req.Host = "192.168.1.5:8722"
	req.Header.Set("Authorization", "Bearer "+a.token)
	local.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("serve phone: %d %s", w.Code, w.Body)
	}
}

func TestSpeechSession(t *testing.T) {
	a := newApp(t.TempDir(), "0")
	defer a.st.Close()
	fake := &fakeEar{text: "今天头痛"}
	a.ear = fake
	h := a.windowHandler(nil)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/assistant/speech/start", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"listening":true`) {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/assistant/speech/start", nil))
	if w.Code == http.StatusOK {
		t.Fatalf("second start should fail: %s", w.Body)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/assistant/speech", nil))
	if !strings.Contains(w.Body.String(), "今天头痛") {
		t.Fatalf("poll: %s", w.Body)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/assistant/speech/stop", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "今天头痛") || strings.Contains(w.Body.String(), `"listening":true`) {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	if fake.starts != 1 {
		t.Fatalf("starts %d", fake.starts)
	}

	old := speechLimit
	speechLimit = 20 * time.Millisecond
	t.Cleanup(func() { speechLimit = old })
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/assistant/speech/start", nil))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body)
	}
	time.Sleep(80 * time.Millisecond)
	if fake.listening() {
		t.Fatal("limit did not stop listening")
	}
}

// Leaving the page (or the window going to the back) drops the HTTP request.
// The draft still finishes, and the page can read it when it comes back.
func TestDraftSurvivesClientLeaving(t *testing.T) {
	a := newApp(t.TempDir(), "0")
	defer a.st.Close()
	h := a.windowHandler(nil)
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce, releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	defer letGo()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startOnce.Do(func() { close(started) })
		<-release
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[{\"kind\":\"record\",\"type\":\"symptoms\",\"fields\":{\"occurred_at\":\"2026-10-27 09:00\",\"description\":\"头痛\",\"severity\":4}}],\"unclear\":\"\"}"}}]}`))
	}))
	defer srv.Close()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/assistant",
		strings.NewReader(`{"base_url":"`+srv.URL+`","api_key":"k","model":"m"}`)))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/draft", strings.NewReader(`{"text":"今天头痛"}`))
	req = req.WithContext(ctx)
	returned := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), req)
		close(returned)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("draft did not reach the model")
	}
	cancel()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("handler kept the dropped request")
	}
	letGo()

	deadline := time.Now().Add(2 * time.Second)
	var body string
	for {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/assistant/draft", nil))
		body = w.Body.String()
		if strings.Contains(body, `"state":"done"`) && strings.Contains(body, "头痛") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("draft after leaving: %s", body)
		}
		time.Sleep(15 * time.Millisecond)
	}
}

type fakeEar struct {
	mu     sync.Mutex
	text   string
	starts int
	on     bool
}

func (f *fakeEar) Available() bool { return true }

func (f *fakeEar) Start(string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.on {
		return speechFailure{Code: 4}
	}
	f.on = true
	f.starts++
	return nil
}

func (f *fakeEar) Stop() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on = false
	return f.text, nil
}

func (f *fakeEar) View() SpeechView {
	f.mu.Lock()
	defer f.mu.Unlock()
	return SpeechView{Listening: f.on, Text: f.text}
}

func (f *fakeEar) listening() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on
}
