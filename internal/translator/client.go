package translator

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Request struct {
	Provider   string
	Endpoint   string
	Model      string
	SecretID   string
	SecretKey  string
	Region     string
	Source     string
	Target     string
	ProjectID  int64
	UserPrompt string
}

type Response struct {
	Text      string
	LatencyMS int64
}

type BatchResponse struct {
	Texts     []string
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

func (c *Client) Translate(ctx context.Context, req Request) (Response, error) {
	provider := normalizeProvider(req.Provider)
	switch provider {
	case "tencent_tmt":
		return c.translateTencent(ctx, req)
	default:
		return Response{}, fmt.Errorf("不支持的翻译 provider：%s", req.Provider)
	}
}

func (c *Client) TranslateBatch(ctx context.Context, req Request, sourceTexts []string) (BatchResponse, error) {
	texts := make([]string, 0, len(sourceTexts))
	for _, t := range sourceTexts {
		if strings.TrimSpace(t) != "" {
			texts = append(texts, t)
		}
	}
	if len(texts) == 0 {
		return BatchResponse{}, fmt.Errorf("批量翻译输入为空")
	}

	provider := normalizeProvider(req.Provider)
	switch provider {
	case "tencent_tmt":
		return c.translateTencentBatch(ctx, req, texts)
	default:
		return BatchResponse{}, fmt.Errorf("不支持的翻译 provider：%s", req.Provider)
	}
}

func normalizeProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "":
		return "tencent_tmt"
	case "tencent", "tencent_tmt":
		return "tencent_tmt"
	default:
		return p
	}
}

func (c *Client) translateTencent(ctx context.Context, req Request) (Response, error) {
	source, target := normalizeLang(req.Source, req.Target)
	payload := map[string]any{
		"SourceText": req.UserPrompt,
		"Source":     source,
		"Target":     target,
		"ProjectId":  req.ProjectID,
	}

	start := time.Now()
	var resp struct {
		Response struct {
			TargetText string `json:"TargetText"`
			Error      *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := c.tencentCall(ctx, req, "TextTranslate", payload, &resp); err != nil {
		return Response{}, err
	}
	if resp.Response.Error != nil {
		return Response{}, fmt.Errorf("腾讯翻译错误 %s: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}
	text := strings.TrimSpace(resp.Response.TargetText)
	if text == "" {
		return Response{}, fmt.Errorf("腾讯翻译返回为空")
	}
	return Response{Text: text, LatencyMS: time.Since(start).Milliseconds()}, nil
}

func (c *Client) translateTencentBatch(ctx context.Context, req Request, sourceTexts []string) (BatchResponse, error) {
	source, target := normalizeLang(req.Source, req.Target)
	payload := map[string]any{
		"SourceTextList": sourceTexts,
		"Source":         source,
		"Target":         target,
		"ProjectId":      req.ProjectID,
	}

	start := time.Now()
	var resp struct {
		Response struct {
			TargetTextList []string `json:"TargetTextList"`
			Error          *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := c.tencentCall(ctx, req, "TextTranslateBatch", payload, &resp); err != nil {
		return BatchResponse{}, err
	}
	if resp.Response.Error != nil {
		return BatchResponse{}, fmt.Errorf("腾讯批量翻译错误 %s: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}
	if len(resp.Response.TargetTextList) != len(sourceTexts) {
		return BatchResponse{}, fmt.Errorf("腾讯批量翻译返回数量不匹配：%d != %d", len(resp.Response.TargetTextList), len(sourceTexts))
	}
	out := make([]string, len(resp.Response.TargetTextList))
	for i, t := range resp.Response.TargetTextList {
		out[i] = strings.TrimSpace(t)
	}
	return BatchResponse{Texts: out, LatencyMS: time.Since(start).Milliseconds()}, nil
}

func normalizeLang(source, target string) (string, string) {
	s := strings.TrimSpace(source)
	t := strings.TrimSpace(target)
	if s == "" {
		s = "en"
	}
	if t == "" {
		t = "zh"
	}
	return s, t
}

func (c *Client) tencentCall(ctx context.Context, req Request, action string, payload map[string]any, out any) error {
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == "" {
		endpoint = "https://tmt.tencentcloudapi.com"
	}
	if strings.TrimSpace(req.SecretID) == "" || strings.TrimSpace(req.SecretKey) == "" {
		return fmt.Errorf("腾讯翻译凭据为空")
	}
	region := strings.TrimSpace(req.Region)
	if region == "" {
		region = "ap-beijing"
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("腾讯翻译 endpoint 无效：%w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Host == "" {
		u.Host = "tmt.tencentcloudapi.com"
	}
	u.Path = "/"
	finalURL := u.String()

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("编码腾讯翻译请求失败：%w", err)
	}
	timestamp := time.Now().Unix()
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	service := "tmt"

	canonicalHeaders := map[string]string{
		"content-type": "application/json; charset=utf-8",
		"host":         u.Host,
		"x-tc-action":  strings.ToLower(action),
		"x-tc-version": "2018-03-21",
		"x-tc-region":  strings.ToLower(region),
	}
	signedHeaders := sortedHeaderKeys(canonicalHeaders)
	canonicalHeadersText := buildCanonicalHeaders(canonicalHeaders, signedHeaders)
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canonicalHeadersText,
		strings.Join(signedHeaders, ";"),
		hashSHA256Hex(body),
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		strconv.FormatInt(timestamp, 10),
		credentialScope,
		hashSHA256Hex([]byte(canonicalRequest)),
	}, "\n")

	secretDate := hmacSHA256([]byte("TC3"+req.SecretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	authorization := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		req.SecretID,
		credentialScope,
		strings.Join(signedHeaders, ";"),
		signature,
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, finalURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建腾讯翻译请求失败：%w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Authorization", authorization)
	httpReq.Header.Set("Host", u.Host)
	httpReq.Header.Set("X-TC-Action", action)
	httpReq.Header.Set("X-TC-Version", "2018-03-21")
	httpReq.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	httpReq.Header.Set("X-TC-Region", region)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("腾讯翻译请求失败：%w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("读取腾讯翻译响应失败：%w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("腾讯翻译 HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("解析腾讯翻译响应失败：%w; 原始响应: %s", err, truncate(string(respBody), 800))
	}
	return nil
}

func sortedHeaderKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, strings.ToLower(strings.TrimSpace(k)))
	}
	sort.Strings(keys)
	return keys
}

func buildCanonicalHeaders(headers map[string]string, signedHeaders []string) string {
	var b strings.Builder
	for _, k := range signedHeaders {
		v := strings.TrimSpace(headers[k])
		b.WriteString(strings.ToLower(k))
		b.WriteString(":")
		b.WriteString(strings.ToLower(v))
		b.WriteByte('\n')
	}
	return b.String()
}

func hmacSHA256(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(msg))
	return h.Sum(nil)
}

func hashSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
