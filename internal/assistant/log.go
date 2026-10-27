package assistant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const assistantLog = "assistant.log"
const assistantLogMax = 2 << 20

var logMu sync.Mutex

// logExchange appends one call to the external model. The key and picture
// bytes stay out; the picture is only named by type and size.
func logExchange(dir string, cfg Config, system string, turns []Turn, reply string, usage Usage, callErr error) {
	if dir == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "—— %s  %s  %s ——\n", time.Now().Format("2006-01-02 15:04:05"), cfg.Model, cfg.BaseURL)
	b.WriteString("--- system ---\n")
	b.WriteString(redact(system, cfg.APIKey))
	b.WriteString("\n")
	for _, t := range turns {
		fmt.Fprintf(&b, "--- %s ---\n", t.Role)
		if t.Text != "" {
			b.WriteString(redact(t.Text, cfg.APIKey))
			b.WriteString("\n")
		}
		for _, im := range t.Images {
			fmt.Fprintf(&b, "[图片 %s，约 %d KB]\n", im.MIME, len(im.Data)*3/4/1024)
		}
	}
	b.WriteString("--- reply ---\n")
	if strings.TrimSpace(reply) == "" {
		b.WriteString("（没有正文）\n")
	} else {
		b.WriteString(redact(reply, cfg.APIKey))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "tokens 输入 %d · 输出 %d · 合计 %d · 缓存命中 %d\n",
		usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, usage.CachedTokens)
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
