// Optional LLM assist for merchants the local rules cannot classify.
//
// It is disabled unless the user supplies their own API key, and it only
// ever sends the merchant name plus the knowledge base's service names —
// never amounts, dates, payment accounts or any other bill content. Cancel
// paths stay human-verified: the LLM only picks from the existing catalog,
// it never invents services or steps.
package match

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Model ids accepted by the DeepSeek-compatible endpoint at the time of
// writing; the id is user-configurable so it can track API changes.
const (
	DefaultModel   = "deepseek-flash"
	DefaultBaseURL = "https://api.deepseek.com"
)

// ErrDisabled is returned when the assist is off or has no API key.
var ErrDisabled = errors.New("match: LLM assist is disabled (no API key configured)")

// LLMConfig enables the optional merchant-classification assist.
type LLMConfig struct {
	Enabled bool
	APIKey  string
	Model   string
	BaseURL string
}

// LLM classifies merchant names through a DeepSeek-compatible
// chat-completions endpoint.
type LLM struct {
	cfg  LLMConfig
	http *http.Client
}

// NewLLM builds the assist client; request timeouts are short because this
// path must never block the local UI for long.
func NewLLM(cfg LLMConfig) *LLM {
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return &LLM{
		cfg:  cfg,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled reports whether classification requests can actually be made.
func (l *LLM) Enabled() bool { return l != nil && l.cfg.Enabled && l.cfg.APIKey != "" }

// Model is the configured model id.
func (l *LLM) Model() string {
	if l == nil || l.cfg.Model == "" {
		return DefaultModel
	}
	return l.cfg.Model
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
	Stream bool `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Classify asks the model to pick the best-fitting canonical service for a
// raw merchant name out of the given candidate names. An empty result means
// the model found no confident match — the UI falls back to "不认识".
func (l *LLM) Classify(ctx context.Context, merchant string, candidates []string) (string, error) {
	if !l.Enabled() {
		return "", ErrDisabled
	}
	if merchant == "" || len(candidates) == 0 {
		return "", nil
	}

	prompt := fmt.Sprintf(
		"账单导出中出现一个商户名称，请从候选服务列表中选出它最可能对应的服务。\n"+
			"商户名称：%s\n候选服务列表：%s\n"+
			`只输出 JSON：{"canonical_name":"候选列表中的某一项"}；若都不匹配则输出 {"canonical_name":""}。`,
		merchant, strings.Join(candidates, "、"),
	)
	req := chatRequest{
		Model:       l.cfg.Model,
		Messages:    []chatMessage{{Role: "user", Content: prompt}},
		Temperature: 0,
		Stream:      false,
	}
	req.ResponseFormat = &struct {
		Type string `json:"type"`
	}{Type: "json_object"}

	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(l.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)

	resp, err := l.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("match: LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()

	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("match: LLM 响应解析失败 (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := ""
		if decoded.Error != nil {
			msg = decoded.Error.Message
		}
		return "", fmt.Errorf("match: LLM 返回 HTTP %d %s", resp.StatusCode, msg)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("match: LLM 未返回任何结果")
	}

	var answer struct {
		CanonicalName string `json:"canonical_name"`
	}
	if err := json.Unmarshal([]byte(decoded.Choices[0].Message.Content), &answer); err != nil {
		return "", fmt.Errorf("match: LLM 输出不是合法 JSON: %w", err)
	}
	guess := strings.TrimSpace(answer.CanonicalName)
	for _, c := range candidates {
		if c == guess {
			return guess, nil // only catalog names may pass through
		}
	}
	return "", nil
}
