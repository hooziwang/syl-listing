package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Request struct {
	Provider        string
	BaseURL         string
	Model           string
	APIMode         string
	APIKey          string
	ReasoningEffort string
	SystemPrompt    string
	UserPrompt      string
	JSONMode        bool
	Timeout         time.Duration
}

type Response struct {
	Text      string
	LatencyMS int64
}

type Client struct {
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Client{httpClient: &http.Client{Timeout: timeout}}
}

func (c *Client) Generate(ctx context.Context, req Request) (Response, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = "openai"
	}
	start := time.Now()
	var (
		text string
		err  error
	)

	switch provider {
	case "openai":
		text, err = c.generateOpenAI(ctx, req)
	case "deepseek":
		text, err = c.generateDeepSeek(ctx, req)
	case "gemini":
		text, err = c.generateGemini(ctx, req)
	case "claude":
		text, err = c.generateClaude(ctx, req)
	default:
		err = fmt.Errorf("不支持的 provider：%s", provider)
	}
	if err != nil {
		return Response{}, err
	}
	return Response{Text: strings.TrimSpace(text), LatencyMS: time.Since(start).Milliseconds()}, nil
}

func (c *Client) generateOpenAI(ctx context.Context, req Request) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(req.APIMode))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "responses":
		return c.openAIResponses(ctx, req)
	case "chat":
		return c.openAIChat(ctx, req)
	case "auto":
		text, err := c.openAIResponses(ctx, req)
		if err == nil {
			return text, nil
		}
		return c.openAIChat(ctx, req)
	default:
		return "", fmt.Errorf("openai api_mode 不支持：%s", req.APIMode)
	}
}

func (c *Client) openAIResponses(ctx context.Context, req Request) (string, error) {
	payload := map[string]any{
		"model": req.Model,
		"input": []map[string]any{
			{
				"role":    "system",
				"content": []map[string]any{{"type": "input_text", "text": req.SystemPrompt}},
			},
			{
				"role":    "user",
				"content": []map[string]any{{"type": "input_text", "text": req.UserPrompt}},
			},
		},
	}
	if strings.TrimSpace(req.ReasoningEffort) != "" {
		payload["reasoning"] = map[string]any{"effort": req.ReasoningEffort}
	}

	var resp struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, joinURL(req.BaseURL, "/v1/responses"), req.APIKey, map[string]string{}, payload, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("responses API 错误：%s", resp.Error.Message)
	}
	if strings.TrimSpace(resp.OutputText) != "" {
		return resp.OutputText, nil
	}
	var b strings.Builder
	for _, o := range resp.Output {
		for _, ctn := range o.Content {
			if strings.TrimSpace(ctn.Text) != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(ctn.Text)
			}
		}
	}
	if strings.TrimSpace(b.String()) == "" {
		return "", fmt.Errorf("responses API 返回为空")
	}
	return b.String(), nil
}

func (c *Client) openAIChat(ctx context.Context, req Request) (string, error) {
	payload := map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
	}
	if strings.TrimSpace(req.ReasoningEffort) != "" {
		payload["reasoning_effort"] = req.ReasoningEffort
		payload["reasoning"] = map[string]any{"effort": req.ReasoningEffort}
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, joinURL(req.BaseURL, "/v1/chat/completions"), req.APIKey, map[string]string{}, payload, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("chat completions 错误：%s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat completions 返回为空")
	}
	text := strings.TrimSpace(resp.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("chat completions 内容为空")
	}
	return text, nil
}

func (c *Client) generateGemini(ctx context.Context, req Request) (string, error) {
	base := strings.TrimSuffix(req.BaseURL, "/")
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return "", fmt.Errorf("gemini model 不能为空")
	}
	u := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", base, url.PathEscape(model), url.QueryEscape(req.APIKey))
	payload := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]string{{"text": req.SystemPrompt}},
		},
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]string{{"text": req.UserPrompt}},
			},
		},
	}

	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, u, "", nil, payload, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("gemini API 错误：%s", resp.Error.Message)
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini 返回为空")
	}
	return resp.Candidates[0].Content.Parts[0].Text, nil
}

func (c *Client) generateClaude(ctx context.Context, req Request) (string, error) {
	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": 4096,
		"system":     req.SystemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": req.UserPrompt},
		},
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	headers := map[string]string{
		"x-api-key":         req.APIKey,
		"anthropic-version": "2023-06-01",
	}
	if err := c.doJSON(ctx, http.MethodPost, joinURL(req.BaseURL, "/v1/messages"), "", headers, payload, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("claude API 错误：%s", resp.Error.Message)
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("claude 返回为空")
	}
	for _, ctn := range resp.Content {
		if strings.TrimSpace(ctn.Text) != "" {
			return ctn.Text, nil
		}
	}
	return "", fmt.Errorf("claude 返回文本为空")
}

func (c *Client) generateDeepSeek(ctx context.Context, req Request) (string, error) {
	payload := map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"temperature": 1.0,
		"stream":      false,
	}
	if req.JSONMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, joinURL(req.BaseURL, "/chat/completions"), req.APIKey, nil, payload, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("deepseek chat completions 错误：%s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("deepseek chat completions 返回为空")
	}
	text := strings.TrimSpace(resp.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("deepseek chat completions 内容为空")
	}
	return text, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint, bearer string, extraHeaders map[string]string, in any, out any) error {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(in); err != nil {
		return fmt.Errorf("编码请求失败：%w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, buf)
	if err != nil {
		return fmt.Errorf("创建请求失败：%w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(bearer) != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败：%w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析响应失败：%w; 原始响应: %s", err, truncate(string(body), 800))
	}
	return nil
}

func joinURL(base, path string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "https://api.openai.com"
	}
	base = strings.TrimSuffix(base, "/")
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	if strings.HasPrefix(path, "/") {
		return base + path
	}
	return base + "/" + path
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
