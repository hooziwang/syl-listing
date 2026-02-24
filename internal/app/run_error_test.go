package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/logging"
	"syl-listing/internal/translator"
)

func TestRunInvalidProviderOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	cfgRoot := filepath.Join(home, ".syl-listing")
	if err := os.MkdirAll(cfgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgRoot, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("provider: deepseek\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(Options{ConfigPath: cfgPath, Provider: "bad", CWD: home})
	if err == nil || !strings.Contains(err.Error(), "配置中不存在 provider") {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestTranslateSectionWithRetryEmptyAndError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":""}}]}`)
	}))
	defer ts.Close()
	tr := translator.NewClient(0)
	text, _, err := translateSectionWithRetry(translateSectionOptions{
		Req:                  listingReqForTest(),
		Section:              "title",
		SourceText:           "hello",
		TranslateProviderCfg: config.ProviderConfig{BaseURL: ts.URL, Model: "deepseek-chat"},
		APIKey:               "k",
		MaxRetries:           0,
		Client:               tr,
		Logger:               &logging.Logger{},
		Candidate:            1,
	})
	if err == nil || text != "" {
		t.Fatalf("expected translate empty error, text=%q err=%v", text, err)
	}
}

func TestTranslateSectionWithRetrySuccessVerbose(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"翻译结果"}}]}`)
	}))
	defer ts.Close()
	tr := translator.NewClient(0)
	var out bytes.Buffer
	logger, _, err := logging.New(&out, "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	text, latency, err := translateSectionWithRetry(translateSectionOptions{
		Req:                  listingReqForTest(),
		Section:              "title",
		SourceText:           "hello",
		TranslateProviderCfg: config.ProviderConfig{BaseURL: ts.URL, Model: "deepseek-chat"},
		APIKey:               "k",
		MaxRetries:           0,
		Client:               tr,
		Logger:               logger,
		Candidate:            1,
	})
	if err != nil || strings.TrimSpace(text) == "" || latency < 0 {
		t.Fatalf("expected success, text=%q latency=%d err=%v", text, latency, err)
	}
	if !strings.Contains(out.String(), "\"event\":\"api_request_translate_title\"") {
		t.Fatalf("expected verbose request log, got: %s", out.String())
	}
}

func listingReqForTest() (req listing.Requirement) {
	req.SourcePath = "/tmp/a.md"
	req.BodyAfterMarker = "body"
	req.Category = "Cat"
	req.Keywords = []string{"alpha", "beta"}
	return
}
