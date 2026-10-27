package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTP client that does not follow redirects, so the key is not sent on to
// another host.
var chatClient = &http.Client{
	Timeout: 60 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func complete(ctx context.Context, cfg Config, system string, turns []Turn) (string, Usage, error) {
	body := chatBody(cfg.Model, system, turns, true)
	content, usage, status, raw, err := postChat(ctx, cfg, body)
	if err != nil {
		return "", Usage{}, err
	}
	if status == http.StatusBadRequest {
		content, usage, status, raw, err = postChat(ctx, cfg, chatBody(cfg.Model, system, turns, false))
		if err != nil {
			return "", Usage{}, err
		}
	}
	if status < 200 || status >= 300 {
		return "", Usage{}, upstreamError(status, raw, cfg.APIKey)
	}
	if strings.TrimSpace(content) == "" {
		return "", Usage{}, errorsNew("接口没有返回内容")
	}
	return content, usage, nil
}

func errorsNew(s string) error { return fmt.Errorf("%s", s) }

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

func chatBody(model, system string, turns []Turn, format bool) []byte {
	msgs := []any{map[string]any{"role": "system", "content": system}}
	for _, t := range turns {
		msgs = append(msgs, map[string]any{"role": t.Role, "content": contentOf(t)})
	}
	m := map[string]any{
		"model":    model,
		"messages": msgs,
	}
	if format {
		m["temperature"] = 0
		m["response_format"] = map[string]string{"type": "json_object"}
	}
	b, _ := json.Marshal(m)
	return b
}

func postChat(ctx context.Context, cfg Config, body []byte) (content string, usage Usage, status int, raw string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, 0, "", fmt.Errorf("没能发起请求")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	res, err := chatClient.Do(req)
	if err != nil {
		return "", Usage{}, 0, "", fmt.Errorf("连不上接口：%s", redact(err.Error(), cfg.APIKey))
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		return "", Usage{}, res.StatusCode, "", fmt.Errorf("读取接口回复失败")
	}
	if len(b) > maxResponse {
		return "", Usage{}, res.StatusCode, "", fmt.Errorf("接口的回复太长")
	}
	raw = string(b)
	content = messageContent(b)
	return content, usageOf(b), res.StatusCode, raw, nil
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

func messageContent(b []byte) string {
	var resp struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(b, &resp) != nil || len(resp.Choices) == 0 {
		return ""
	}
	switch c := resp.Choices[0].Message.Content.(type) {
	case string:
		return c
	case []any:
		var s strings.Builder
		for _, p := range c {
			m, _ := p.(map[string]any)
			if t, _ := m["text"].(string); t != "" {
				s.WriteString(t)
			}
		}
		return s.String()
	default:
		return ""
	}
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
