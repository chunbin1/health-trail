package assistant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const assistantLog = "assistant.log"
const assistantLogMax = 2 << 20

var logMu sync.Mutex

// logExchange appends one reply to the log: when, which model, which tools
// it called, how many tokens, and the error. What the person said, the
// archive and the answer stay out; they are health data.
func logExchange(dir string, cfg Config, d Draft, callErr error) {
	if dir == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "—— %s  %s  %s ——\n", time.Now().Format("2006-01-02 15:04:05"), cfg.Model, cfg.BaseURL)
	if len(d.Steps) > 0 {
		names := make([]string, len(d.Steps))
		for i, s := range d.Steps {
			names[i] = s.Tool
			if s.Error {
				names[i] += "（出错）"
			}
		}
		b.WriteString("工具：" + strings.Join(names, "、") + "\n")
	}
	fmt.Fprintf(&b, "回复 %d 字 · 整理 %d 条\n", utf8.RuneCountInString(d.Reply), len(d.Items))
	fmt.Fprintf(&b, "tokens 输入 %d · 输出 %d · 合计 %d · 缓存命中 %d\n",
		d.Usage.PromptTokens, d.Usage.CompletionTokens, d.Usage.TotalTokens, d.Usage.CachedTokens)
	if callErr != nil {
		fmt.Fprintf(&b, "--- error ---\n%s\n", redact(callErr.Error(), cfg.APIKey))
	}
	b.WriteString("\n")

	logMu.Lock()
	defer logMu.Unlock()
	path := filepath.Join(dir, assistantLog)
	rotateLog(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(b.String())
	_ = f.Close()
}

func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < assistantLogMax {
		return
	}
	_ = os.Rename(path, path+".1")
}
