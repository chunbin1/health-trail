package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lee/health-trail/internal/msg"
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

var errNoTools = msg.New("这个模型不支持工具调用，没法查档案。请在接口设置里换一个支持工具调用（function calling）的模型", "This model can't call tools, so it can't read your records. Choose a model that supports tool calls (function calling) in the API settings")

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
		return message{}, Usage{}, msg.New("接口没有返回内容", "The API returned nothing")
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
		return nil, 0, msg.New("没能发起请求", "Couldn't send the request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	res, err := chatClient.Do(req)
	if err != nil {
		return nil, 0, msg.Errorf("连不上接口：%s", "Couldn't reach the API: %s", redact(err.Error(), cfg.APIKey))
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		return nil, res.StatusCode, msg.New("读取接口回复失败", "Couldn't read the API's reply")
	}
	if len(b) > maxResponse {
		return nil, res.StatusCode, msg.New("接口的回复太长", "The API's reply is too long")
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
		return message{}, msg.New("接口的回复读不懂", "Can't understand the API's reply")
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
		return message{}, msg.New("接口的回复读不懂", "Can't understand the API's reply")
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
	detail, code := raw, ""
	var wrap struct {
		Error any `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &wrap) == nil && wrap.Error != nil {
		switch e := wrap.Error.(type) {
		case string:
			detail = e
		case map[string]any:
			if m, _ := e["message"].(string); m != "" {
				detail = m
			}
			code = errorCode(e)
		}
	}
	detail = strings.TrimSpace(redact(detail, key))
	detail = strings.Join(strings.Fields(detail), " ")
	if len(detail) > 200 {
		detail = detail[:200]
	}
	head := msg.Errorf("接口返回了错误（%d）", "The API returned an error (%d)", status)
	if zh, en := hint(status, code, detail); zh != "" {
		head = msg.Errorf(zh+"（%d）", en+" (%d)", status)
	}
	if detail == "" {
		return head
	}
	return msg.Errorf("%s：%s", "%s: %s", head, detail)
}

func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "…")
}
