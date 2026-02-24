package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIChatAndResponsesErrorBranches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			fmt.Fprint(w, `{"choices":[]}`)
		case "/v1/responses":
			fmt.Fprint(w, `{"output":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClient(0)
	_, err := c.Generate(context.Background(), Request{Provider: "openai", APIMode: "chat", BaseURL: ts.URL, Model: "m", APIKey: "k", UserPrompt: "u"})
	if err == nil || !strings.Contains(err.Error(), "返回为空") {
		t.Fatalf("expected openai chat empty choices error, got %v", err)
	}
	_, err = c.Generate(context.Background(), Request{Provider: "openai", APIMode: "responses", BaseURL: ts.URL, Model: "m", APIKey: "k", UserPrompt: "u"})
	if err == nil || !strings.Contains(err.Error(), "返回为空") {
		t.Fatalf("expected openai responses empty error, got %v", err)
	}
}

func TestOpenAIChatErrorMessageAndEmptyContent(t *testing.T) {
	msgErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":{"message":"boom"}}`)
	}))
	defer msgErr.Close()
	c := NewClient(0)
	_, err := c.Generate(context.Background(), Request{Provider: "openai", APIMode: "chat", BaseURL: msgErr.URL, Model: "m", APIKey: "k", UserPrompt: "u"})
	if err == nil || !strings.Contains(err.Error(), "chat completions 错误") {
		t.Fatalf("expected chat error field branch, got %v", err)
	}

	emptyContent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"  "}}]}`)
	}))
	defer emptyContent.Close()
	_, err = c.Generate(context.Background(), Request{Provider: "openai", APIMode: "chat", BaseURL: emptyContent.URL, Model: "m", APIKey: "k", UserPrompt: "u"})
	if err == nil || !strings.Contains(err.Error(), "内容为空") {
		t.Fatalf("expected chat empty content branch, got %v", err)
	}
}

func TestDeepSeekClaudeGeminiErrorBranches(t *testing.T) {
	deepseekEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"   "}}]}`)
	}))
	defer deepseekEmpty.Close()
	c := NewClient(0)
	_, err := c.Generate(context.Background(), Request{
		Provider:   "deepseek",
		BaseURL:    deepseekEmpty.URL,
		Model:      "deepseek-chat",
		APIKey:     "k",
		UserPrompt: "u",
	})
	if err == nil || !strings.Contains(err.Error(), "内容为空") {
		t.Fatalf("expected deepseek empty content error, got %v", err)
	}

	claudeEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"content":[{"type":"text","text":"  "}]} `)
	}))
	defer claudeEmpty.Close()
	_, err = c.Generate(context.Background(), Request{
		Provider:   "claude",
		BaseURL:    claudeEmpty.URL,
		Model:      "claude",
		APIKey:     "k",
		UserPrompt: "u",
	})
	if err == nil || !strings.Contains(err.Error(), "文本为空") {
		t.Fatalf("expected claude empty text error, got %v", err)
	}

	claudeNoContent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"content":[]}`)
	}))
	defer claudeNoContent.Close()
	_, err = c.Generate(context.Background(), Request{
		Provider:   "claude",
		BaseURL:    claudeNoContent.URL,
		Model:      "claude",
		APIKey:     "k",
		UserPrompt: "u",
	})
	if err == nil || !strings.Contains(err.Error(), "claude 返回为空") {
		t.Fatalf("expected claude empty content branch, got %v", err)
	}

	_, err = c.Generate(context.Background(), Request{
		Provider: "gemini",
		BaseURL:  "https://example.com",
		Model:    "",
		APIKey:   "k",
	})
	if err == nil || !strings.Contains(err.Error(), "model 不能为空") {
		t.Fatalf("expected gemini model empty error, got %v", err)
	}
}
