package assistant

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/lee/health-trail/internal/msg"
)

// Provider is a service AI助手 can be set up with in a few clicks: the
// settings page fills in the address and model, and links to where a key is
// made. Adding one is an entry in Providers; the page lists them in order,
// and any other service is still typed in by hand.
type Provider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameEN  string `json:"name_en"`
	BaseURL string `json:"base_url"`
	// Models are offered in a list; the first is the default. Only models
	// that read images: AI助手 is sent photos of reports and prescriptions.
	// Any other model name can still be typed in.
	Models []ModelOption `json:"models"`
	// KeyPage is where a signed-in person creates a key.
	KeyPage string `json:"key_page"`
	// Signup registers a new account. When Invite is set it carries the
	// author's invite code, and the page says so next to the link.
	Signup string `json:"signup"`
	Invite bool   `json:"invite"`
}

// ModelOption is a model in a provider's list, with a few words on price.
type ModelOption struct {
	ID     string `json:"id"`
	Note   string `json:"note"`
	NoteEN string `json:"note_en"`
}

// Providers is what the settings page offers.
var Providers = []Provider{
	{
		ID:      "zhipu",
		Name:    "智谱",
		NameEN:  "Zhipu",
		BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		// docs.bigmodel.cn/cn/guide/start/pricing, 2026-10-30
		Models: []ModelOption{
			{"glm-4.6v-flash", "免费；用的人多时可能要过一会儿再试", "Free; when it's busy you may have to try again later"},
			{"glm-4.6v", "很便宜，免费版的正式版", "Very cheap; the full version of the free one"},
			{"glm-5.3-flash", "很便宜，新一代；每次先思考，回得慢一些", "Very cheap, newer; thinks first, so replies are slower"},
			{"glm-5v-turbo", "较贵", "Pricier"},
		},
		KeyPage: "https://bigmodel.cn/usercenter/proj-mgmt/apikeys",
		Signup:  "https://www.bigmodel.cn/invite?icode=mQwuPZJbfFIn%2B%2BkOjEbLXwZ3c5owLmCCcMQXWcJRS8E%3D",
		Invite:  true,
	},
}

// ProviderPage is the page of a listed provider to open: "signup" or "key".
// Only addresses from Providers are ever returned.
func ProviderPage(id, page string) (string, bool) {
	for _, p := range Providers {
		if p.ID != id {
			continue
		}
		switch page {
		case "signup":
			return p.Signup, p.Signup != ""
		case "key":
			return p.KeyPage, p.KeyPage != ""
		}
	}
	return "", false
}

// Check sends one short message with the saved settings and one tool, so a
// wrong key, a model that can't call tools or an empty balance shows up
// right after saving, not on the first real record.
func Check(ctx context.Context, cfg Config) error {
	if !cfg.Configured() {
		return msg.New("还没有填完接口地址、密钥和模型", "The API address, key and model aren't all filled in yet")
	}
	msgs := []any{
		map[string]any{"role": "system", "content": "只回复 OK。"},
		map[string]any{"role": "user", "content": "OK"},
	}
	tools := []any{map[string]any{"type": "function", "function": map[string]any{
		"name":        "ping",
		"description": "不用调用。",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}}
	_, _, err := chat(ctx, cfg, msgs, tools, "auto")
	return err
}

// hint says in plain words what an error from the service most likely
// means, in Chinese and English. Codes are Zhipu's
// (docs.bigmodel.cn/cn/faq/api-code); the rest goes by the HTTP status and
// the usual OpenAI wording.
func hint(status int, code, detail string) (zh, en string) {
	low := strings.ToLower(detail)
	switch {
	case code == "1113" || strings.Contains(detail, "欠费") || strings.Contains(detail, "余额") ||
		strings.Contains(low, "insufficient") || strings.Contains(low, "balance") || strings.Contains(low, "quota"):
		return "账户余额不足，请到服务商那里充值，或者换一个免费模型",
			"Your balance has run out; top up with the provider or switch to a free model"
	case code == "1315":
		return "这个密钥不是按量付费的 API 密钥（套餐的密钥不能用），请在服务商的 API Key 页面新建一个",
			"This isn't a pay-as-you-go API key (plan keys don't work here); create one on the provider's API Key page"
	case status == http.StatusUnauthorized || code == "1000" || code == "1001" || code == "1003":
		return "密钥不对或已经失效，请重新复制一个完整的密钥",
			"The key is wrong or no longer valid; copy the whole key again"
	case code == "1211" || strings.Contains(detail, "模型不存在") ||
		(strings.Contains(low, "model") && (strings.Contains(low, "not found") || strings.Contains(low, "not exist"))):
		return "模型名不对，请检查模型名", "The model name is wrong; check it"
	case status == http.StatusNotFound:
		return "接口地址不对，请检查地址", "The API address is wrong; check it"
	case code == "1305" || strings.Contains(detail, "访问量过大"):
		return "这个模型现在用的人太多（免费模型常这样），请稍后再试，或者换一个付费模型",
			"Too many people are using this model right now (free models often are); try again later or switch to a paid model"
	case status == http.StatusTooManyRequests:
		return "请求太频繁或服务商正忙，请稍后再试", "Too many requests, or the provider is busy; try again later"
	}
	return "", ""
}

func errorCode(e map[string]any) string {
	switch c := e["code"].(type) {
	case string:
		return c
	case float64:
		return fmt.Sprint(int(c))
	}
	return ""
}
