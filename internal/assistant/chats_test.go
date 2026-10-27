package assistant

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestChatsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	first := json.RawMessage(`[{"role":"user","text":"给我推荐用药"},{"role":"assistant","reply":"可以考虑非布司他"}]`)
	if _, err := SaveChat(dir, "a", first); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("尿", 50)
	second := json.RawMessage(`[{"role":"user","text":"` + long + `"}]`)
	if _, err := SaveChat(dir, "b", second); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveChat(dir, "a", first); err != nil {
		t.Fatal(err)
	}
	list := ListChats(dir)
	if len(list) != 2 || list[0].ID != "b" || list[1].ID != "a" {
		t.Fatalf("list %+v", list)
	}
	if list[0].Messages != nil {
		t.Fatal("list should omit messages")
	}
	if !strings.HasSuffix(list[0].Title, "…") || len([]rune(list[0].Title)) > 41 {
		t.Fatalf("title %q", list[0].Title)
	}
	got, ok := GetChat(dir, "b")
	if !ok || !strings.Contains(string(got.Messages), "尿") {
		t.Fatalf("get %+v %v", got, ok)
	}
	for i := 0; i < chatsKept+5; i++ {
		if _, err := SaveChat(dir, "n"+strconv.Itoa(i), json.RawMessage(`[{"role":"user","text":"x"}]`)); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(ListChats(dir)); n != chatsKept {
		t.Fatalf("kept %d", n)
	}
	used := json.RawMessage(`[{"role":"assistant","usage":{"total_tokens":15}},{"role":"assistant","usage":{"total_tokens":25}}]`)
	saved, err := SaveChat(dir, "tokens", used)
	if err != nil || saved.Tokens != 40 {
		t.Fatalf("tokens %+v %v", saved, err)
	}
	if ListChats(dir)[0].Tokens != 40 {
		t.Fatal("list dropped the token total")
	}
}
