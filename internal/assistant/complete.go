package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTP client that does not follow redirects, so the key is not sent on to
// another host. One call can take a while when the model reads a picture or
// thinks before it answers.
var chatClient = &http.Client{
	Timeout: 120 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// message is one reply from the model: text, tool calls, or both. Raw is the
// message as the service sent it; it goes back unchanged in the next request,
// with any fields the service needs to see again (reasoning_content, say).
type message struct {
	Content string
	Calls   []toolCall
	Raw     json.RawMessage
}

type toolCall struct {
	ID   string
	Name string
	Args string
}

var errNoTools = errors.New("这个模型不支持工具调用，没法查档案。请在接口设置里换一个支持工具调用（function calling）的模型")

// chat sends the conversation with the tools. choice is "auto", or "none" on
// the last round so the model has to answer.
func chat(ctx context.Context, cfg Config, msgs []any, tools []any, choice string) (message, Usage, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       cfg.Model,
		"messages":    msgs,
		"tools":       tools,
		"tool_choice": choice,
	})
	b, status, err := postChat(ctx, cfg, body)
	if err != nil {
		return message{}, Usage{}, err
	}
	if status < 200 || status >= 300 {
		err := upstreamError(status, string(b), cfg.APIKey)
		if status == http.StatusBadRequest && strings.Contains(strings.ToLower(err.Error()), "tool") {
			return message{}, Usage{}, errNoTools
		}
		return message{}, Usage{}, err
	}
	m, err := messageOf(b)
	if err != nil {
		return message{}, Usage{}, err
	}
	if strings.TrimSpace(m.Content) == "" && len(m.Calls) == 0 {
		return message{}, Usage{}, errors.New("接口没有返回内容")
	}
	return m, usageOf(b), nil
}

func contentOf(t Turn) any {
	if t.Role != "user" || len(t.Images) == 0 {
		return t.Text
	}
	parts := []any{}
	if t.Text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": t.Text})
	} else {
		parts = append(parts, map[string]any{"type": "text", "text": "请整理这张图片。"})
	}
	for _, im := range t.Images {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": "data:" + im.MIME + ";base64," + im.Data},
		})
	}
	return parts
}

func postChat(ctx context.Context, cfg Config, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("没能发起请求")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	res, err := chatClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("连不上接口：%s", redact(err.Error(), cfg.APIKey))
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("读取接口回复失败")
	}
	if len(b) > maxResponse {
		return nil, res.StatusCode, fmt.Errorf("接口的回复太长")
	}
	return b, res.StatusCode, nil
}

func usageOf(b []byte) Usage {
	var resp struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
			Details          struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(b, &resp) != nil {
		return Usage{}
	}
	u := Usage{
		PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens: resp.Usage.TotalTokens, CachedTokens: resp.Usage.Details.CachedTokens,
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

func messageOf(b []byte) (message, error) {
	var resp struct {
		Choices []struct {
			Message json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(b, &resp) != nil || len(resp.Choices) == 0 || len(resp.Choices[0].Message) == 0 {
		return message{}, errors.New("接口的回复读不懂")
	}
	raw := resp.Choices[0].Message
	var m struct {
		Content   any `json:"content"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return message{}, errors.New("接口的回复读不懂")
	}
	out := message{Raw: raw}
	switch c := m.Content.(type) {
	case string:
		out.Content = c
	case []any:
		var s strings.Builder
		for _, p := range c {
			part, _ := p.(map[string]any)
			if t, _ := part["text"].(string); t != "" {
				s.WriteString(t)
			}
		}
		out.Content = s.String()
	}
	for _, c := range m.ToolCalls {
		out.Calls = append(out.Calls, toolCall{ID: c.ID, Name: c.Function.Name, Args: c.Function.Arguments})
	}
	return out, nil
}

func upstreamError(status int, raw, key string) error {
	msg := raw
	var wrap struct {
		Error any `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &wrap) == nil && wrap.Error != nil {
		switch e := wrap.Error.(type) {
		case string:
			msg = e
		case map[string]any:
			if m, _ := e["message"].(string); m != "" {
				msg = m
			}
		}
	}
	msg = strings.TrimSpace(redact(msg, key))
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 200 {
		msg = msg[:200]
	}
	if msg == "" {
		return fmt.Errorf("接口返回了错误（%d）", status)
	}
	return fmt.Errorf("接口返回了错误（%d）：%s", status, msg)
}

func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "…")
}
