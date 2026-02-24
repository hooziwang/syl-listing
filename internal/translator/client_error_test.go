package translator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateDeepSeekErrorBranches(t *testing.T) {
	emptyChoices := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer emptyChoices.Close()
	c := NewClient(0)
	_, err := c.Translate(context.Background(), Request{
		Provider:   "deepseek",
		Endpoint:   emptyChoices.URL,
		Model:      "deepseek-chat",
		APIKey:     "k",
		UserPrompt: "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "返回为空") {
		t.Fatalf("expected deepseek empty choices error, got %v", err)
	}

	emptyContent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"  "}}]}`)
	}))
	defer emptyContent.Close()
	_, err = c.Translate(context.Background(), Request{
		Provider:   "deepseek",
		Endpoint:   emptyContent.URL,
		Model:      "deepseek-chat",
		APIKey:     "k",
		UserPrompt: "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "内容为空") {
		t.Fatalf("expected deepseek empty content error, got %v", err)
	}
}

func TestTencentBatchAndDoJSONErrorBranches(t *testing.T) {
	tencentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-TC-Action") {
		case "TextTranslateBatch":
			fmt.Fprint(w, `{"Response":{"TargetTextList":["only-one"]}}`)
		default:
			http.Error(w, "bad action", http.StatusBadRequest)
		}
	}))
	defer tencentSrv.Close()

	c := NewClient(0)
	_, err := c.TranslateBatch(context.Background(), Request{
		Provider:  "tencent",
		Endpoint:  tencentSrv.URL,
		SecretID:  "id",
		SecretKey: "key",
		Region:    "ap-beijing",
	}, []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "数量不匹配") {
		t.Fatalf("expected tencent batch mismatch error, got %v", err)
	}

	invalidJSONSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{bad-json"))
	}))
	defer invalidJSONSrv.Close()
	var out map[string]any
	err = c.doJSON(context.Background(), http.MethodPost, invalidJSONSrv.URL, "k", map[string]any{"x": 1}, &out)
	if err == nil || !strings.Contains(err.Error(), "解析响应失败") {
		t.Fatalf("expected doJSON unmarshal error, got %v", err)
	}
}
