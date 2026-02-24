package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
)

func TestGenerateBulletsBatchJSONWithRetry_ParseThenRecover(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"not-json"}}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"bullets\":[\"bullet one enough\",\"bullet two enough\",\"bullet three enough\",\"bullet four enough\",\"bullet five enough\"]}"}}]}`)
	}))
	defer ts.Close()

	rule := testRules().Bullets
	req := listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "body", Category: "Cat", Keywords: []string{"alpha", "beta", "gamma"}}
	items, _, err := generateBulletsBatchJSONWithRetry(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		Provider:      "deepseek",
		ProviderCfg:   config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		APIKey:        "k",
		Rules:         testRules(),
		MaxRetries:    1,
		Client:        llm.NewClient(10 * time.Second),
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}, ListingDocument{Category: "Cat", Keywords: req.Keywords}, rule, resolveCharBounds(10, 40, 0))
	if err != nil {
		t.Fatalf("expected recover after parse retry, got %v", err)
	}
	if len(items) != 5 || calls < 2 {
		t.Fatalf("unexpected items/calls: len=%d calls=%d", len(items), calls)
	}
}

func TestRegenerateBulletItemJSONWithRetry_JSONParseFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"not-json"}}]}`)
	}))
	defer ts.Close()

	req := listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "body", Category: "Cat", Keywords: []string{"alpha", "beta"}}
	_, _, err := regenerateBulletItemJSONWithRetry(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		Provider:      "deepseek",
		ProviderCfg:   config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		APIKey:        "k",
		Rules:         testRules(),
		MaxRetries:    0,
		Client:        llm.NewClient(10 * time.Second),
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}, ListingDocument{Category: "Cat", Keywords: req.Keywords}, testRules().Bullets, 1, []string{"a", "b", "c", "d", "e"}, resolveCharBounds(10, 40, 0))
	if err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("expected json parse failure, got %v", err)
	}
}

func TestGenerateBulletsWithJSONAndRepair_FailedRepair(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		user := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(user, "【子任务】只修复第") {
			// Always still too long, so repair keeps failing.
			fmt.Fprintf(w, `{"choices":[{"message":{"content":"{\"bullet\":%q}"}}]}`, strings.Repeat("x", 120))
			return
		}
		// Initial batch is valid JSON but all lines too long.
		long := strings.Repeat("x", 120)
		fmt.Fprintf(w, `{"choices":[{"message":{"content":"{\"bullets\":[%q,%q,%q,%q,%q]}"}}]}`, long, long, long, long, long)
	}))
	defer ts.Close()

	req := listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "body", Category: "Cat", Keywords: []string{"alpha", "beta"}}
	_, _, err := generateBulletsWithJSONAndRepair(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		Provider:      "deepseek",
		ProviderCfg:   config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		APIKey:        "k",
		Rules:         testRules(),
		MaxRetries:    0,
		Client:        llm.NewClient(10 * time.Second),
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}, ListingDocument{Category: "Cat", Keywords: req.Keywords, Title: "alpha beta title"}, testRules().Bullets)
	if err == nil || !strings.Contains(err.Error(), "重试后仍失败") {
		t.Fatalf("expected repair failure, got %v", err)
	}
}
