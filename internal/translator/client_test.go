package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeProviderAndLang(t *testing.T) {
	if normalizeProvider("") != "tencent_tmt" || normalizeProvider("tencent") != "tencent_tmt" || normalizeProvider("deepseek") != "deepseek" {
		t.Fatalf("normalizeProvider mismatch")
	}
	s, t2 := normalizeLang("", "")
	if s != "en" || t2 != "zh" {
		t.Fatalf("normalizeLang mismatch")
	}
}

func TestTranslateDeepSeekAndBatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		msgs := req["messages"].([]any)
		user := msgs[len(msgs)-1].(map[string]any)["content"].(string)
		fmt.Fprintf(w, `{"choices":[{"message":{"content":"中:%s"}}]}`+"\n", user)
	}))
	defer ts.Close()

	c := NewClient(0)
	one, err := c.Translate(context.Background(), Request{Provider: "deepseek", Endpoint: ts.URL, APIKey: "k", Model: "deepseek-chat", UserPrompt: "hello"})
	if err != nil || one.Text != "中:hello" {
		t.Fatalf("Translate failed: %+v err=%v", one, err)
	}
	batch, err := c.TranslateBatch(context.Background(), Request{Provider: "deepseek", Endpoint: ts.URL, APIKey: "k", Model: "deepseek-chat"}, []string{"a", "", "b"})
	if err != nil || len(batch.Texts) != 2 || batch.Texts[0] != "中:a" || batch.Texts[1] != "中:b" {
		t.Fatalf("TranslateBatch failed: %+v err=%v", batch, err)
	}
}

func TestTranslateErrors(t *testing.T) {
	c := NewClient(0)
	_, err := c.Translate(context.Background(), Request{Provider: "deepseek", Endpoint: "https://api.deepseek.com", APIKey: ""})
	if err == nil || !strings.Contains(err.Error(), "API key 为空") {
		t.Fatalf("expected api key error, got %v", err)
	}
	_, err = c.TranslateBatch(context.Background(), Request{Provider: "deepseek"}, []string{"", " "})
	if err == nil {
		t.Fatalf("expected empty batch error")
	}
}

func TestTencentHelpers(t *testing.T) {
	headers := map[string]string{"Host": "A.COM", "x-tc-action": "TextTranslate"}
	keys := sortedHeaderKeys(headers)
	if len(keys) != 2 || keys[0] != "host" {
		t.Fatalf("sortedHeaderKeys mismatch: %+v", keys)
	}
	canon := buildCanonicalHeaders(map[string]string{"host": "A.COM"}, []string{"host"})
	if canon != "host:a.com\n" {
		t.Fatalf("canonical headers mismatch: %q", canon)
	}
	if hashSHA256Hex([]byte("x")) == "" || len(hmacSHA256([]byte("k"), "m")) == 0 {
		t.Fatalf("hash/hmac mismatch")
	}
	if truncate("abc", 2) != "ab" {
		t.Fatalf("truncate mismatch")
	}
	if joinURL("https://a.com/", "/x") != "https://a.com/x" {
		t.Fatalf("joinURL mismatch")
	}
}
