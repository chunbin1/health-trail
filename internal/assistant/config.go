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
)

// Config is the saved interface. APIKey is never returned by the HTTP API.
type Config struct {
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Model   string `json:"model,omitempty"`
	path    string
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
			return c, errors.New("密钥无效")
		}
		next.APIKey = k
	}
	if err := next.save(); err != nil {
		return c, err
	}
	return next, nil
}

func (c Config) save() error {
	if c.path == "" {
		return errors.New("没有配置文件路径")
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
		return "", errors.New("请填写接口地址")
	}
	if len(raw) > 300 {
		return "", errors.New("接口地址太长")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return "", errors.New("接口地址无法识别")
	}
	if u.User != nil {
		return "", errors.New("接口地址里不要写账号密码")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !localHost(u.Hostname()) {
			return "", errors.New("http 只允许本机地址（127.0.0.1、localhost）；其他地址请用 https")
		}
	default:
		return "", errors.New("接口地址只支持 https，本机可以用 http")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("接口地址不要带 ? 或 #")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, "/chat/completions")
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String(), nil
}

func localHost(h string) bool {
	h = strings.Trim(strings.ToLower(h), "[]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// CleanModel checks a model name.
func CleanModel(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("请填写模型名")
	}
	if len(s) > 80 || strings.ContainsAny(s, "\r\n\t") {
		return "", errors.New("模型名无效")
	}
	return s, nil
}
