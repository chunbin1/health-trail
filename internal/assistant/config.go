// Package assistant turns a sentence into health records through an
// OpenAI-compatible chat API the person configures. The key stays in a
// file on this computer.
package assistant

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/lee/health-trail/internal/msg"
)

// Config is the saved interface. APIKey is never returned by the HTTP API.
type Config struct {
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Model   string `json:"model,omitempty"`
	path    string
	// Lang is the UI's language for what the person sees ("en" or "zh");
	// not saved
	Lang string `json:"-"`
}

func configPath(dir string) string { return filepath.Join(dir, "assistant.json") }

// Load reads assistant.json. A missing file is an empty config.
func Load(dir string) (Config, error) {
	c := Config{path: configPath(dir)}
	b, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	c.path = configPath(dir)
	return c, nil
}

// Configured is true when a draft can be sent.
func (c Config) Configured() bool {
	return c.BaseURL != "" && c.APIKey != "" && c.Model != ""
}

// Update changes the saved fields. An empty APIKey keeps the old key,
// unless clearKey is set. An empty base URL or model clears that field.
func (c Config) Update(baseURL, apiKey, model *string, clearKey bool) (Config, error) {
	next := c
	if baseURL != nil {
		if strings.TrimSpace(*baseURL) == "" {
			next.BaseURL = ""
		} else {
			u, err := NormalizeBaseURL(*baseURL)
			if err != nil {
				return c, err
			}
			next.BaseURL = u
		}
	}
	if model != nil {
		if strings.TrimSpace(*model) == "" {
			next.Model = ""
		} else {
			m, err := CleanModel(*model)
			if err != nil {
				return c, err
			}
			next.Model = m
		}
	}
	if clearKey {
		next.APIKey = ""
	} else if apiKey != nil && strings.TrimSpace(*apiKey) != "" {
		k := strings.TrimSpace(*apiKey)
		if len(k) > 500 || strings.ContainsAny(k, "\r\n") {
			return c, msg.New("密钥无效", "The key isn't valid")
		}
		next.APIKey = k
	}
	// the saved key belongs to the old host: going to another one takes a key
	// filled in this time (the same one is fine), so it isn't sent on by itself
	typed := apiKey != nil && strings.TrimSpace(*apiKey) != ""
	if to := hostOf(next.BaseURL); to != "" && to != hostOf(c.BaseURL) && next.APIKey != "" && !typed {
		return c, msg.New("换了接口地址，请重新填写密钥", "The address changed; enter the key again")
	}
	if err := next.save(); err != nil {
		return c, err
	}
	return next, nil
}

func (c Config) save() error {
	if c.path == "" {
		return msg.New("没有配置文件路径", "No settings file path")
	}
	b, err := json.MarshalIndent(struct {
		BaseURL string `json:"base_url,omitempty"`
		APIKey  string `json:"api_key,omitempty"`
		Model   string `json:"model,omitempty"`
	}{c.BaseURL, c.APIKey, c.Model}, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// NormalizeBaseURL checks the address the chat request will use.
// http is only allowed for this computer; anything else must be https.
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", msg.New("请填写接口地址", "Enter the API address")
	}
	if len(raw) > 300 {
		return "", msg.New("接口地址太长", "The API address is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return "", msg.New("接口地址无法识别", "Can't read the API address")
	}
	if u.User != nil {
		return "", msg.New("接口地址里不要写账号密码", "Don't put a user name or password in the API address")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !localHost(u.Hostname()) {
			return "", msg.New("http 只允许本机地址（127.0.0.1、localhost）；其他地址请用 https", "http is only for this computer (127.0.0.1, localhost); use https for other addresses")
		}
	default:
		return "", msg.New("接口地址只支持 https，本机可以用 http", "The API address must be https (http only for this computer)")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", msg.New("接口地址不要带 ? 或 #", "The API address can't have ? or #")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, "/chat/completions")
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String(), nil
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func localHost(h string) bool {
	h = strings.Trim(strings.ToLower(h), "[]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// CleanModel checks a model name.
func CleanModel(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", msg.New("请填写模型名", "Enter the model name")
	}
	if len(s) > 80 || strings.ContainsAny(s, "\r\n\t") {
		return "", msg.New("模型名无效", "The model name isn't valid")
	}
	return s, nil
}
