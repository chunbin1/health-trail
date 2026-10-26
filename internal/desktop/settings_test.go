package desktop

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each list keeps its own order and what is off; one put leaves the others
// alone, and the 今天 page's choice from before lists carries over.
func TestLists(t *testing.T) {
	dir := t.TempDir()
	// as an older version saved it
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"lan":false,"hidden_metrics":["体温"]}`), 0o600)
	a := newApp(dir, "0")
	defer a.st.Close()
	put := func(body string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body)))
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	lists := func(out map[string]any) string {
		b, _ := json.Marshal(out["lists"])
		return string(b)
	}
	if _, out := put(`{}`); lists(out) != `{"today_metrics":{"hidden":["体温"],"order":[]}}` {
		t.Errorf("before any list: %s, want the old 今天 choice", lists(out))
	}
	_, out := put(`{"lists":{"metrics_page":{"order":["体重"," 体重 ","BMI"],"hidden":["腰围"]}}}`)
	if lists(out) != `{"metrics_page":{"hidden":["腰围"],"order":["体重","BMI"]},"today_metrics":{"hidden":["体温"],"order":[]}}` {
		t.Errorf("after the 身体数据 page: %s", lists(out))
	}
	if _, out := put(`{"lists":{"today_metrics":{"order":["BMI","体重"],"hidden":[]}}}`); !strings.Contains(lists(out), `"today_metrics":{"hidden":[],"order":["BMI","体重"]}`) || !strings.Contains(lists(out), `"metrics_page"`) {
		t.Errorf("after 今天: %s", lists(out))
	}
	for _, bad := range []string{`{"lists":{"Today":{"order":[]}}}`, `{"lists":{"a b":{"order":[]}}}`} {
		if code, out := put(bad); code != 400 || out["error"] == nil {
			t.Errorf("%s: %d %v, want a 400 saying why", bad, code, out)
		}
	}
	// kept across a restart, the old field gone once carried over
	b := newApp(dir, "0")
	defer b.st.Close()
	if got := b.settings.lists()["today_metrics"]; len(got.Hidden) != 0 || len(got.Order) != 2 {
		t.Errorf("after restart: %v", got)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "settings.json")); strings.Contains(string(raw), "hidden_metrics") {
		t.Errorf("settings.json still has hidden_metrics: %s", raw)
	}
}
