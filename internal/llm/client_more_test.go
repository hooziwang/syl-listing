package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateOpenAIResponsesAndChat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			fmt.Fprint(w, `{"output_text":"resp-text"}`)
		case "/v1/chat/completions":
			fmt.Fprint(w, `{"choices":[{"message":{"content":"chat-text"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClient(0)
	resp, err := c.Generate(context.Background(), Request{Provider: "openai", APIMode: "responses", BaseURL: ts.URL, Model: "m", APIKey: "k", SystemPrompt: "s", UserPrompt: "u"})
	if err != nil || resp.Text != "resp-text" {
		t.Fatalf("responses mode failed: resp=%+v err=%v", resp, err)
	}
	resp, err = c.Generate(context.Background(), Request{Provider: "openai", APIMode: "chat", BaseURL: ts.URL, Model: "m", APIKey: "k", SystemPrompt: "s", UserPrompt: "u"})
	if err != nil || resp.Text != "chat-text" {
		t.Fatalf("chat mode failed: resp=%+v err=%v", resp, err)
	}
}

func TestGenerateOpenAIAutoFallbackToChat(t *testing.T) {
	var responseCalls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			responseCalls++
			http.Error(w, "bad", http.StatusBadRequest)
		case "/v1/chat/completions":
			fmt.Fprint(w, `{"choices":[{"message":{"content":"chat-text"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClient(0)
	resp, err := c.Generate(context.Background(), Request{Provider: "openai", APIMode: "auto", BaseURL: ts.URL, Model: "m", APIKey: "k", SystemPrompt: "s", UserPrompt: "u"})
	if err != nil || resp.Text != "chat-text" {
		t.Fatalf("auto fallback failed: resp=%+v err=%v", resp, err)
	}
	if responseCalls == 0 {
		t.Fatalf("expected responses attempt before fallback")
	}
}

func TestOpenAIResponsesOutputFromArray(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"output":[{"content":[{"type":"output_text","text":"a"},{"type":"output_text","text":"b"}]}]}`)
	}))
	defer ts.Close()

	c := NewClient(0)
	resp, err := c.Generate(context.Background(), Request{Provider: "openai", APIMode: "responses", BaseURL: ts.URL, Model: "m", APIKey: "k", SystemPrompt: "s", UserPrompt: "u"})
	if err != nil || resp.Text != "a\nb" {
		t.Fatalf("responses output array failed: resp=%+v err=%v", resp, err)
	}
}

func TestGenerateGeminiAndClaudeAndUnsupported(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, ":generateContent"):
			fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"gemini-text"}]}}]}`)
		case r.URL.Path == "/v1/messages":
			fmt.Fprint(w, `{"content":[{"type":"text","text":"claude-text"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClient(0)
	resp, err := c.Generate(context.Background(), Request{Provider: "gemini", BaseURL: ts.URL, Model: "gemini-2.5-pro", APIKey: "k", SystemPrompt: "s", UserPrompt: "u"})
	if err != nil || resp.Text != "gemini-text" {
		t.Fatalf("gemini failed: resp=%+v err=%v", resp, err)
	}
	resp, err = c.Generate(context.Background(), Request{Provider: "claude", BaseURL: ts.URL, Model: "claude", APIKey: "k", SystemPrompt: "s", UserPrompt: "u"})
	if err != nil || resp.Text != "claude-text" {
		t.Fatalf("claude failed: resp=%+v err=%v", resp, err)
	}
	_, err = c.Generate(context.Background(), Request{Provider: "unknown"})
	if err == nil || !strings.Contains(err.Error(), "不支持的 provider") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestGenerateOpenAIUnsupportedMode(t *testing.T) {
	c := NewClient(0)
	_, err := c.generateOpenAI(context.Background(), Request{APIMode: "x"})
	if err == nil {
		t.Fatalf("expected api_mode error")
	}
}

func TestGeminiEmptyCandidatesError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{}})
	}))
	defer ts.Close()
	c := NewClient(0)
	_, err := c.Generate(context.Background(), Request{Provider: "gemini", BaseURL: ts.URL, Model: "m", APIKey: "k"})
	if err == nil {
		t.Fatalf("expected gemini empty error")
	}
}
