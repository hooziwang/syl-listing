package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
)

func newLLMTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) == 0 {
			http.Error(w, "empty", http.StatusBadRequest)
			return
		}
		user := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(user, "【子任务】只修复第") {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"bullet\":\"fixed bullet line enough\"}"}}]}`)
			return
		}
		re := regexp.MustCompile(`【当前任务】生成：([^\n]+)`)
		m := re.FindStringSubmatch(user)
		step := ""
		if len(m) == 2 {
			step = strings.TrimSpace(m[1])
		}
		var out string
		switch step {
		case "title":
			out = "alpha beta generated title"
		case "bullets":
			out = `{"bullets":["bullet line 1 enough","bullet line 2 enough","bullet line 3 enough","bullet line 4 enough","bullet line 5 enough"]}`
		case "description":
			out = "desc paragraph one.\n\ndesc paragraph two."
		case "search_terms":
			out = "alpha beta gamma"
		default:
			out = "fallback"
		}
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, out)
	}))
}

func TestGenerateDocumentBySectionsDeepSeek(t *testing.T) {
	ts := newLLMTestServer()
	defer ts.Close()
	client := llm.NewClient(10 * time.Second)

	rules := testRules()
	req := listing.Requirement{
		SourcePath:      "/tmp/a.md",
		BodyAfterMarker: "body",
		Category:        "Cat",
		Keywords:        []string{"alpha", "beta", "gamma"},
	}
	doc, _, err := generateDocumentBySections(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		CharTolerance: 20,
		Provider:      "deepseek",
		ProviderCfg: config.ProviderConfig{
			BaseURL: ts.URL,
			APIMode: "chat",
			Model:   "deepseek-chat",
		},
		APIKey:     "k",
		Rules:      rules,
		MaxRetries: 1,
		Client:     client,
		Logger:     nil,
		Candidate:  1,
	})
	if err != nil {
		t.Fatalf("generateDocumentBySections error: %v", err)
	}
	if doc.Title == "" || len(doc.BulletPoints) != 5 || len(doc.DescriptionParagraphs) != 2 || doc.SearchTerms == "" {
		t.Fatalf("unexpected doc: %+v", doc)
	}
}

func TestGenerateDocumentBySectionsOpenAI(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		user := req.Messages[len(req.Messages)-1].Content
		re := regexp.MustCompile(`【当前任务】生成：([^\n]+)`)
		m := re.FindStringSubmatch(user)
		step := ""
		if len(m) == 2 {
			step = strings.TrimSpace(m[1])
		}
		out := "fallback"
		switch step {
		case "title":
			out = "alpha beta openai title"
		case "bullets":
			out = "1) bullet line 1 enough\n2) bullet line 2 enough\n3) bullet line 3 enough\n4) bullet line 4 enough\n5) bullet line 5 enough"
		case "description":
			out = "desc one.\n\ndesc two."
		case "search_terms":
			out = "alpha beta gamma"
		}
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, out)
	}))
	defer ts.Close()
	client := llm.NewClient(10 * time.Second)

	rules := testRules()
	rules.Bullets.Parsed.Execution.Generation.Protocol = "text"
	rules.Bullets.Parsed.Execution.Repair.Granularity = "whole"
	req := listing.Requirement{
		SourcePath:      "/tmp/a.md",
		BodyAfterMarker: "body",
		Category:        "Cat",
		Keywords:        []string{"alpha", "beta", "gamma"},
	}
	doc, _, err := generateDocumentBySections(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		CharTolerance: 20,
		Provider:      "openai",
		ProviderCfg: config.ProviderConfig{
			BaseURL: ts.URL,
			APIMode: "chat",
			Model:   "gpt-4o-mini",
		},
		APIKey:     "k",
		Rules:      rules,
		MaxRetries: 1,
		Client:     client,
		Logger:     nil,
		Candidate:  1,
	})
	if err != nil {
		t.Fatalf("generateDocumentBySections openai error: %v", err)
	}
	if doc.Title == "" || len(doc.BulletPoints) != 5 || len(doc.DescriptionParagraphs) != 2 || doc.SearchTerms == "" {
		t.Fatalf("unexpected doc: %+v", doc)
	}
}

func TestRegenerateBulletItemJSONWithRetry(t *testing.T) {
	ts := newLLMTestServer()
	defer ts.Close()
	client := llm.NewClient(10 * time.Second)
	rule := testRules().Bullets
	req := listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "body", Category: "Cat", Keywords: []string{"alpha", "beta"}}
	doc := ListingDocument{Title: "alpha beta title", BulletPoints: []string{"short", "b2", "b3", "b4", "b5"}}
	line, _, err := regenerateBulletItemJSONWithRetry(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		Provider:      "deepseek",
		ProviderCfg:   config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		APIKey:        "k",
		Rules:         testRules(),
		MaxRetries:    1,
		Client:        client,
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}, doc, rule, 1, doc.BulletPoints, resolveCharBounds(10, 40, 0))
	if err != nil {
		t.Fatalf("regenerateBulletItemJSONWithRetry error: %v", err)
	}
	if strings.TrimSpace(line) == "" {
		t.Fatalf("expected repaired line")
	}
}

func TestGenerateSectionWithRetryValidationFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"no keyword"}}]}`)
	}))
	defer ts.Close()
	client := llm.NewClient(10 * time.Second)
	rules := testRules()
	req := listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "body", Category: "Cat", Keywords: []string{"alpha", "beta"}}
	_, _, err := generateSectionWithRetry(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		Provider:      "deepseek",
		ProviderCfg:   config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		APIKey:        "k",
		Rules:         rules,
		MaxRetries:    0,
		Client:        client,
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}, "title", ListingDocument{Category: "Cat", Keywords: req.Keywords})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestGenerateBulletsWithJSONAndRepair(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		user := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(user, "【子任务】只修复第") {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"bullet\":\"fixed bullet line enough\"}"}}]}`)
			return
		}
		// Intentionally return oversize bullet lines to trigger item repairs.
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"bullets\":[\"`+strings.Repeat("x", 60)+`\",\"`+strings.Repeat("x", 60)+`\",\"`+strings.Repeat("x", 60)+`\",\"`+strings.Repeat("x", 60)+`\",\"`+strings.Repeat("x", 60)+`\"]}"}}]}`)
	}))
	defer ts.Close()

	client := llm.NewClient(10 * time.Second)
	rule := config.SectionRuleFile{
		Raw: "rule",
		Parsed: config.SectionRule{
			Output: config.RuleOutputSpec{Format: "json_object", Lines: 5},
			Constraints: config.RuleConstraints{
				MinCharsPerLine: config.RuleIntConstraint{Value: 10},
				MaxCharsPerLine: config.RuleIntConstraint{Value: 40},
			},
			Execution: config.RuleExecutionSpec{
				Generation: config.RuleGenerationSpec{Protocol: "json_lines"},
				Repair:     config.RuleRepairPolicySpec{Granularity: "item", ItemJSONField: "item"},
				Fallback:   config.RuleFallbackPolicySpec{DisableThinkingOnLengthError: boolPtrApp(true)},
			},
		},
	}
	req := listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "body", Category: "Cat", Keywords: []string{"alpha", "beta"}}
	items, _, err := generateBulletsWithJSONAndRepair(sectionGenerateOptions{
		Req:           req,
		Lang:          "en",
		Provider:      "deepseek",
		ProviderCfg:   config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		APIKey:        "k",
		Rules:         testRules(),
		MaxRetries:    1,
		Client:        client,
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}, ListingDocument{Category: "Cat", Keywords: req.Keywords, Title: "alpha beta title"}, rule)
	if err != nil {
		t.Fatalf("generateBulletsWithJSONAndRepair error: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 repaired items, got %d", len(items))
	}
	for _, it := range items {
		if strings.TrimSpace(it) == "" {
			t.Fatalf("empty repaired item")
		}
	}
}
