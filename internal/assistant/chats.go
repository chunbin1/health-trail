package assistant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const chatsFile = "chats.json"
const chatsKept = 80

var chatsMu sync.Mutex

// Chat is one assistant conversation, kept on this computer so it can be
// opened again, including the pictures that were sent.
type Chat struct {
	ID       string          `json:"id"`
	Updated  string          `json:"updated"`
	Title    string          `json:"title"`
	Tokens   int             `json:"tokens,omitempty"`
	Messages json.RawMessage `json:"messages,omitempty"`
}

type chatFile struct {
	Chats []Chat `json:"chats"`
}

// SaveChat inserts or replaces one conversation. An existing one stays where
// it is; only a new one is added at the front. The list keeps chatsKept.
func SaveChat(dir, id string, messages json.RawMessage) (Chat, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Chat{}, errChat("没有对话编号")
	}
	if !json.Valid(messages) || len(messages) == 0 || messages[0] != '[' {
		return Chat{}, errChat("对话内容无法保存")
	}
	chatsMu.Lock()
	defer chatsMu.Unlock()
	file := readChats(dir)
	now := time.Now().Format("2006-01-02 15:04")
	next := Chat{ID: id, Updated: now, Title: chatTitle(messages), Tokens: chatTokens(messages), Messages: messages}
	out := make([]Chat, 0, len(file.Chats)+1)
	replaced := false
	for _, c := range file.Chats {
		if c.ID == id {
			out = append(out, next)
			replaced = true
			continue
		}
		out = append(out, c)
	}
	if !replaced {
		out = append([]Chat{next}, out...)
	}
	if len(out) > chatsKept {
		out = out[:chatsKept]
	}
	if err := writeChats(dir, chatFile{Chats: out}); err != nil {
		return Chat{}, err
	}
	next.Messages = nil
	return next, nil
}

// ListChats returns the newest conversations without their messages.
func ListChats(dir string) []Chat {
	chatsMu.Lock()
	defer chatsMu.Unlock()
	file := readChats(dir)
	out := make([]Chat, len(file.Chats))
	for i, c := range file.Chats {
		c.Messages = nil
		out[i] = c
	}
	return out
}

// GetChat returns one conversation, including its messages.
func GetChat(dir, id string) (Chat, bool) {
	chatsMu.Lock()
	defer chatsMu.Unlock()
	for _, c := range readChats(dir).Chats {
		if c.ID == id {
			return c, true
		}
	}
	return Chat{}, false
}

func chatTitle(messages json.RawMessage) string {
	var msgs []struct {
		Role string `json:"role"`
		Text string `json:"text"`
	}
	if json.Unmarshal(messages, &msgs) != nil {
		return "对话"
	}
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "" {
			s := strings.TrimSpace(m.Text)
			if s == "" {
				return "（图片）"
			}
			if utf8.RuneCountInString(s) > 40 {
				s = string([]rune(s)[:40]) + "…"
			}
			return s
		}
	}
	return "对话"
}

func chatTokens(messages json.RawMessage) int {
	var msgs []struct {
		Usage struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(messages, &msgs) != nil {
		return 0
	}
	n := 0
	for _, m := range msgs {
		n += m.Usage.Total
	}
	return n
}

func errChat(s string) error { return &chatError{s} }

type chatError struct{ s string }

func (e *chatError) Error() string { return e.s }

func readChats(dir string) chatFile {
	b, err := os.ReadFile(filepath.Join(dir, chatsFile))
	if err != nil {
		return chatFile{}
	}
	var file chatFile
	if json.Unmarshal(b, &file) != nil {
		return chatFile{}
	}
	return file
}

func writeChats(dir string, file chatFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, chatsFile), b, 0o600)
}
