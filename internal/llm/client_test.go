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

func TestResolveMessages(t *testing.T) {
	msgs := resolveMessages(Request{Messages: []Message{{Role: "", Content: " hi "}, {Role: "assistant", Content: ""}}})
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "hi" {
		t.Fatalf("unexpected msgs: %+v", msgs)
	}
	msgs = resolveMessages(Request{SystemPrompt: "s", UserPrompt: "u"})
	if len(msgs) != 2 {
		t.Fatalf("unexpected fallback msgs: %+v", msgs)
	}
}

func TestJoinURLAndTruncate(t *testing.T) {
	if got := joinURL("https://a.com/v1", "/v1/chat/completions"); got != "https://a.com/v1/chat/completions" {
		t.Fatalf("joinURL mismatch: %s", got)
	}
	if got := joinURL("", "/v1/x"); !strings.HasPrefix(got, "https://api.openai.com") {
		t.Fatalf("joinURL default mismatch: %s", got)
	}
	if truncate("abcdef", 3) != "abc" {
		t.Fatalf("truncate mismatch")
	}
}

func TestGenerateDeepSeek(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if _, ok := req["response_format"]; !ok {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"k\":1}"}}]}`)
	}))
	defer ts.Close()

	c := NewClient(0)
	resp, err := c.Generate(context.Background(), Request{
		Provider:     "deepseek",
		BaseURL:      ts.URL,
		Model:        "deepseek-chat",
		APIKey:       "x",
		SystemPrompt: "s",
		UserPrompt:   "u",
	})
	if err != nil || resp.Text != "ok" {
		t.Fatalf("Generate deepseek failed: resp=%+v err=%v", resp, err)
	}
	resp, err = c.Generate(context.Background(), Request{
		Provider:     "deepseek",
		BaseURL:      ts.URL,
		Model:        "deepseek-chat",
		APIKey:       "x",
		SystemPrompt: "s",
		UserPrompt:   "u",
		JSONMode:     true,
	})
	if err != nil || !strings.Contains(resp.Text, "\"k\":1") {
		t.Fatalf("Generate deepseek json failed: resp=%+v err=%v", resp, err)
	}
}

func TestDoJSONHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer ts.Close()

	c := NewClient(0)
	var out map[string]any
	err := c.doJSON(context.Background(), http.MethodPost, ts.URL, "", nil, map[string]any{"a": 1}, &out)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected http error, got %v", err)
	}
}
