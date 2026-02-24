package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/logging"
	"syl-listing/internal/translator"
)

func TestProcessCandidateSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(raw, &req)
		user := req.Messages[len(req.Messages)-1].Content
		sys := req.Messages[0].Content
		if strings.Contains(sys, "你是专业翻译") {
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, "中:"+user)
			return
		}
		re := regexp.MustCompile(`【当前任务】生成：([^\n]+)`)
		m := re.FindStringSubmatch(user)
		step := ""
		if len(m) == 2 {
			step = strings.TrimSpace(m[1])
		}
		out := "fallback"
		switch step {
		case "title":
			out = "alpha beta title"
		case "bullets":
			out = `{"bullets":["bullet one enough","bullet two enough","bullet three enough","bullet four enough","bullet five enough"]}`
		case "description":
			out = "desc one.\n\ndesc two."
		case "search_terms":
			out = "alpha beta gamma"
		}
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, out)
	}))
	defer ts.Close()

	logger, _, err := logging.New(io.Discard, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	ok := processCandidate(processCandidateOptions{
		Job: candidateJob{
			Req: listing.Requirement{
				SourcePath:      "/tmp/a.md",
				BodyAfterMarker: "body",
				Brand:           "BrandX",
				Category:        "Cat",
				Keywords:        []string{"alpha", "beta"},
			},
			Candidate: 1,
		},
		OutDir:               work,
		CharTolerance:        20,
		Provider:             "deepseek",
		ProviderCfg:          config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		TranslateProviderCfg: config.ProviderConfig{BaseURL: ts.URL, Model: "deepseek-chat"},
		APIKey:               "k",
		Rules:                testRules(),
		MaxRetries:           0,
		Client:               llm.NewClient(10 * time.Second),
		TranslateClient:      translator.NewClient(10 * time.Second),
		Logger:               logger,
	})
	if !ok {
		t.Fatalf("expected processCandidate success")
	}
	enFiles, _ := filepath.Glob(filepath.Join(work, "listing_*_en.md"))
	cnFiles, _ := filepath.Glob(filepath.Join(work, "listing_*_cn.md"))
	if len(enFiles) != 1 || len(cnFiles) != 1 {
		t.Fatalf("expected 1 en + 1 cn file, got en=%d cn=%d", len(enFiles), len(cnFiles))
	}
}

func TestProcessCandidateGenerateFailed(t *testing.T) {
	logger, _, err := logging.New(io.Discard, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	ok := processCandidate(processCandidateOptions{
		Job: candidateJob{
			Req: listing.Requirement{
				SourcePath:      "/tmp/a.md",
				BodyAfterMarker: "body",
				Brand:           "BrandX",
				Category:        "Cat",
				Keywords:        []string{"alpha", "beta"},
			},
			Candidate: 1,
		},
		OutDir:               t.TempDir(),
		CharTolerance:        20,
		Provider:             "deepseek",
		ProviderCfg:          config.ProviderConfig{BaseURL: "http://127.0.0.1:1", APIMode: "chat", Model: "deepseek-chat"},
		TranslateProviderCfg: config.ProviderConfig{BaseURL: "http://127.0.0.1:1", Model: "deepseek-chat"},
		APIKey:               "k",
		Rules:                testRules(),
		MaxRetries:           0,
		Client:               llm.NewClient(100 * time.Millisecond),
		TranslateClient:      translator.NewClient(100 * time.Millisecond),
		Logger:               logger,
	})
	if ok {
		t.Fatalf("expected processCandidate generate failure")
	}
}
