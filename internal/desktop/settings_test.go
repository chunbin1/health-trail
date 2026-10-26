package desktop

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// The 身体数据 page's choice is its own: unset (null) until made, kept apart
// from the 今天 page's, and an empty choice stays a choice.
func TestMetricsPageHidden(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, "0")
	defer a.st.Close()
	put := func(body string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body)))
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("PUT %s: %d %s", body, w.Code, w.Body)
		}
		return out
	}
	if got := put(`{}`)["metrics_page_hidden"]; got != nil {
		t.Errorf("before a choice: %v, want null", got)
	}
	out := put(`{"metrics_page_hidden":["腰围"," 腰围 ","步数"]}`)
	if got, _ := json.Marshal(out["metrics_page_hidden"]); string(got) != `["腰围","步数"]` || out["hidden_metrics"] != nil && len(out["hidden_metrics"].([]any)) != 0 {
		t.Errorf("after choosing: page %s, 今天 %v", got, out["hidden_metrics"])
	}
	if got, _ := json.Marshal(put(`{"metrics_page_hidden":[]}`)["metrics_page_hidden"]); string(got) != `[]` {
		t.Errorf("nothing hidden: %s, want [] (a choice, not null)", got)
	}
	// kept in settings.json across a restart
	b := newApp(dir, "0")
	defer b.st.Close()
	if got := b.settings.pageHidden(); got == nil || len(got) != 0 {
		t.Errorf("after restart: %v, want []", got)
	}
}
