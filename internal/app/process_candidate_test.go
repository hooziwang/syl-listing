package app

import (
	"io"
	"path/filepath"
	"testing"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/logging"
	"syl-listing/internal/translator"
)

func TestProcessCandidateWriteFailed(t *testing.T) {
	ts := newLLMTestServer()
	defer ts.Close()
	llmClient := llm.NewClient(10 * time.Second)
	trClient := translator.NewClient(10 * time.Second)
	logger, _, err := logging.New(io.Discard, "", false, false)
	if err != nil {
		t.Fatal(err)
	}

	req := listing.Requirement{
		SourcePath:      "/tmp/a.md",
		BodyAfterMarker: "body",
		Brand:           "BrandX",
		Category:        "Cat",
		Keywords:        []string{"alpha", "beta", "gamma"},
	}

	ok := processCandidate(processCandidateOptions{
		Job:           candidateJob{Req: req, Candidate: 1},
		OutDir:        filepath.Join(t.TempDir(), "not-exist", "dir"),
		CharTolerance: 20,
		Provider:      "deepseek",
		ProviderCfg: config.ProviderConfig{
			BaseURL: ts.URL,
			APIMode: "chat",
			Model:   "deepseek-chat",
		},
		TranslateProviderCfg: config.ProviderConfig{BaseURL: ts.URL, Model: "deepseek-chat"},
		APIKey:               "k",
		Rules:                testRules(),
		MaxRetries:           0,
		Client:               llmClient,
		TranslateClient:      trClient,
		Logger:               logger,
	})
	if ok {
		t.Fatalf("expected processCandidate failure when output dir missing")
	}
}
