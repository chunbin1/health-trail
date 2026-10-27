package assistant

import (
	"encoding/json"
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

// a tool result is shown up to here in the log; the model got all of it
const logResultMax = 600

var logMu sync.Mutex

// exchange is what one reply sent and got back, for the log.
type exchange struct {
	system  int // characters of the system prompt
	tools   int // characters of the tool definitions
	toolN   int
	history int // turns before the latest one
	ask     Turn
	rounds  []round
}

// round is one request: what it cost, what the model wrote and which tools
// it called.
type round struct {
	usage Usage
	said  string
	calls []loggedCall
}

type loggedCall struct {
	name, args, out string
	failed          bool
}

func newExchange(system string, defs []any, turns []Turn) *exchange {
	x := &exchange{system: utf8.RuneCountInString(system), toolN: len(defs)}
	if b, err := json.Marshal(defs); err == nil {
		x.tools = utf8.RuneCount(b)
	}
	if len(turns) > 0 {
		x.ask = turns[len(turns)-1]
		x.history = len(turns) - 1
	}
	return x
}

func (x *exchange) request(u Usage, said string) {
	x.rounds = append(x.rounds, round{usage: u, said: strings.TrimSpace(said)})
}

func (x *exchange) call(name, args, out string, failed bool) {
	if len(x.rounds) == 0 {
		return
	}
	r := &x.rounds[len(x.rounds)-1]
	r.calls = append(r.calls, loggedCall{name, strings.TrimSpace(args), out, failed})
}

// logExchange appends one reply to the log in the data folder, next to
// chats.json: what was asked, each request with its tokens, the tools the
// model called with what they returned (cut short), the answer and the
// cards. The system prompt and earlier turns are only counted, and pictures
// only named; the key never goes in.
func logExchange(dir string, cfg Config, x *exchange, d Draft, callErr error) {
	if dir == "" || x == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "—— %s  %s  %s ——\n", time.Now().Format("2006-01-02 15:04:05"), cfg.Model, cfg.BaseURL)
	fmt.Fprintf(&b, "每次请求都带：系统提示 %d 字 · 工具说明 %d 字（%d 个工具）· 之前的对话 %d 段\n",
		x.system, x.tools, x.toolN, x.history)
	b.WriteString("--- 用户 ---\n")
	if x.ask.Text != "" {
		b.WriteString(x.ask.Text + "\n")
	}
	for _, im := range x.ask.Images {
		fmt.Fprintf(&b, "（图片 %s，约 %d KB）\n", im.MIME, len(im.Data)*3/4>>10)
	}
	for i, r := range x.rounds {
		fmt.Fprintf(&b, "--- 第 %d 次请求 · 输入 %d · 输出 %d · 缓存命中 %d ---\n",
			i+1, r.usage.PromptTokens, r.usage.CompletionTokens, r.usage.CachedTokens)
		if r.said != "" {
			b.WriteString("模型：" + r.said + "\n")
		}
		for _, c := range r.calls {
			args := c.args
			if args == "" {
				args = "{}"
			}
			fmt.Fprintf(&b, "调用 %s %s\n", c.name, args)
			mark := "→ "
			if c.failed {
				mark = "→ 出错："
			}
			out := strings.ReplaceAll(strings.TrimSpace(c.out), "\n", "\n    ")
			if n := utf8.RuneCountInString(c.out); n > logResultMax {
				out = clip(out, logResultMax) + fmt.Sprintf("（共 %d 字）", n)
			}
			fmt.Fprintf(&b, "  %s%s\n", mark, out)
		}
	}
	if d.Reply != "" {
		b.WriteString("--- 回复 ---\n" + d.Reply + "\n")
	}
	if len(d.Items) > 0 {
		b.WriteString("--- 卡片 ---\n")
		for i, it := range d.Items {
			j, _ := json.Marshal(it)
			fmt.Fprintf(&b, "%d. %s\n", i+1, j)
		}
	}
	if d.Unclear != "" {
		b.WriteString("--- 没整理出来 ---\n" + d.Unclear + "\n")
	}
	fmt.Fprintf(&b, "tokens 合计：%d 次请求 · 输入 %d · 输出 %d · 合计 %d · 缓存命中 %d\n",
		len(x.rounds), d.Usage.PromptTokens, d.Usage.CompletionTokens, d.Usage.TotalTokens, d.Usage.CachedTokens)
	if callErr != nil {
		fmt.Fprintf(&b, "--- 出错 ---\n%s\n", callErr.Error())
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
	_, _ = f.WriteString(redact(b.String(), cfg.APIKey))
	_ = f.Close()
}

func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < assistantLogMax {
		return
	}
	_ = os.Rename(path, path+".1")
}
