package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const deepSeekBalanceURL = "https://api.deepseek.com/user/balance"

type deepSeekBalanceResponse struct {
	IsAvailable bool                  `json:"is_available"`
	BalanceInfo []deepSeekBalanceInfo `json:"balance_infos"`
	Error       *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type deepSeekBalanceInfo struct {
	Currency      any `json:"currency"`
	TotalBalance  any `json:"total_balance"`
	GrantedAmount any `json:"granted_balance"`
	ToppedUp      any `json:"topped_up_balance"`
}

func resolveDeepSeekBalanceKey(envMap map[string]string, apiKey string) string {
	if key := strings.TrimSpace(apiKey); key != "" {
		return key
	}
	if envMap != nil {
		if key := strings.TrimSpace(envMap["DEEPSEEK_API_KEY"]); key != "" {
			return key
		}
	}
	return ""
}

func fetchDeepSeekBalanceWithRetry(apiKey string, maxRetries int) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("未找到 DEEPSEEK_API_KEY")
	}

	var balance string
	err := withExponentialBackoff(retryOptions{
		MaxRetries: maxRetries,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Jitter:     0.2,
	}, func(attempt int) error {
		out, err := fetchDeepSeekBalance(apiKey)
		if err != nil {
			return err
		}
		balance = out
		return nil
	})
	if err != nil {
		return "", err
	}
	return balance, nil
}

func fetchDeepSeekBalance(apiKey string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, deepSeekBalanceURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建余额请求失败：%w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("余额请求失败：%w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("读取余额响应失败：%w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("余额接口返回 %d：%s", resp.StatusCode, shortBody(body))
	}

	var parsed deepSeekBalanceResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("解析余额响应失败：%w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", fmt.Errorf("余额接口错误：%s", strings.TrimSpace(parsed.Error.Message))
	}

	balance := formatDeepSeekBalance(parsed.BalanceInfo)
	if balance == "" {
		return "", fmt.Errorf("余额接口返回为空")
	}
	return balance, nil
}

func formatDeepSeekBalance(items []deepSeekBalanceInfo) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		currency := strings.ToUpper(strings.TrimSpace(anyToString(item.Currency)))
		if currency == "" {
			currency = "UNKNOWN"
		}
		total := firstNonEmpty(
			anyToString(item.TotalBalance),
			anyToString(item.ToppedUp),
			anyToString(item.GrantedAmount),
		)
		total = strings.TrimSpace(total)
		if total == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", currency, total))
	}
	return strings.Join(parts, " | ")
}

func formatBalanceForSummary(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "查询失败"
	}
	parts := strings.Split(trimmed, "|")
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) >= 2 && strings.EqualFold(fields[0], "CNY") {
			return strings.Join(fields[1:], " ") + " 元"
		}
	}
	return trimmed
}

func anyToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmt.Sprintf("%g", x)
	case int:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func shortBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "-"
	}
	if len(s) > 280 {
		return s[:280] + "..."
	}
	return s
}
