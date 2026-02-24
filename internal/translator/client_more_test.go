package translator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateTencentAndBatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("X-TC-Action")
		switch action {
		case "TextTranslate":
			fmt.Fprint(w, `{"Response":{"TargetText":"中文单条"}}`)
		case "TextTranslateBatch":
			fmt.Fprint(w, `{"Response":{"TargetTextList":["中1","中2"]}}`)
		default:
			http.Error(w, "bad action", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	c := NewClient(0)
	resp, err := c.Translate(context.Background(), Request{Provider: "tencent", Endpoint: ts.URL, SecretID: "id", SecretKey: "key", Region: "ap-beijing", UserPrompt: "hello"})
	if err != nil || resp.Text != "中文单条" {
		t.Fatalf("Translate tencent failed: resp=%+v err=%v", resp, err)
	}
	batch, err := c.TranslateBatch(context.Background(), Request{Provider: "tencent", Endpoint: ts.URL, SecretID: "id", SecretKey: "key", Region: "ap-beijing"}, []string{"a", "b"})
	if err != nil || len(batch.Texts) != 2 || batch.Texts[0] != "中1" {
		t.Fatalf("TranslateBatch tencent failed: %+v err=%v", batch, err)
	}
}

func TestTencentErrorBranches(t *testing.T) {
	c := NewClient(0)
	_, err := c.Translate(context.Background(), Request{Provider: "tencent", Endpoint: "https://example.com", SecretID: "", SecretKey: ""})
	if err == nil || !strings.Contains(err.Error(), "凭据为空") {
		t.Fatalf("expected credential error, got %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "oops", http.StatusBadRequest)
	}))
	defer bad.Close()
	_, err = c.Translate(context.Background(), Request{Provider: "tencent", Endpoint: bad.URL, SecretID: "id", SecretKey: "key"})
	if err == nil {
		t.Fatalf("expected http error")
	}
}
