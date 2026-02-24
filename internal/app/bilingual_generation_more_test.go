package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/translator"
)

func TestGenerateENAndTranslateCNBySections_OpenAIPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions", "/chat/completions":
			raw, _ := io.ReadAll(r.Body)
			body := string(raw)
			if strings.Contains(body, `"role":"system","content":"你是专业翻译`) {
				var req struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				_ = json.Unmarshal(raw, &req)
				user := req.Messages[len(req.Messages)-1].Content
				fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, "中:"+user)
				return
			}
			var req struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			_ = json.Unmarshal(raw, &req)
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
				out = "alpha beta title"
			case "bullets":
				out = "1) bullet 1 enough\n2) bullet 2 enough\n3) bullet 3 enough\n4) bullet 4 enough\n5) bullet 5 enough"
			case "description":
				out = "desc one.\n\ndesc two."
			case "search_terms":
				out = "alpha beta gamma"
			}
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	req := listing.Requirement{
		SourcePath:      "/tmp/a.md",
		BodyAfterMarker: "body",
		Category:        "Cat",
		Keywords:        []string{"alpha", "beta"},
	}
	en, cn, enMS, cnMS, err := generateENAndTranslateCNBySections(bilingualGenerateOptions{
		Req:                  req,
		CharTolerance:        20,
		Provider:             "openai",
		ProviderCfg:          config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "gpt"},
		TranslateProviderCfg: config.ProviderConfig{BaseURL: ts.URL, Model: "deepseek-chat"},
		APIKey:               "k",
		Rules:                testRules(),
		MaxRetries:           0,
		Client:               llm.NewClient(10 * time.Second),
		TranslateClient:      translator.NewClient(10 * time.Second),
		Logger:               nil,
		Candidate:            1,
	})
	if err != nil {
		t.Fatalf("expected success on openai path, got %v", err)
	}
	if en.Title == "" || len(en.BulletPoints) != 5 || len(en.DescriptionParagraphs) != 2 || strings.TrimSpace(en.SearchTerms) == "" {
		t.Fatalf("invalid en doc: %+v", en)
	}
	if cn.Title == "" || strings.TrimSpace(cn.Category) == "" || len(cn.Keywords) != len(req.Keywords) {
		t.Fatalf("invalid cn doc: %+v", cn)
	}
	if enMS < 0 || cnMS < 0 {
		t.Fatalf("latency should be non-negative: en=%d cn=%d", enMS, cnMS)
	}
}

func TestGenerateENAndTranslateCNBySections_CNTitleValidationError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
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
		sys := req.Messages[0].Content
		if strings.Contains(sys, "你是专业翻译") {
			if strings.TrimSpace(user) == "alpha beta title" {
				fmt.Fprint(w, `{"choices":[{"message":{"content":"标题:"}}]}`)
				return
			}
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

	_, _, _, _, err := generateENAndTranslateCNBySections(bilingualGenerateOptions{
		Req: listing.Requirement{
			SourcePath:      "/tmp/a.md",
			BodyAfterMarker: "body",
			Category:        "Cat",
			Keywords:        []string{"alpha", "beta"},
		},
		CharTolerance:        20,
		Provider:             "deepseek",
		ProviderCfg:          config.ProviderConfig{BaseURL: ts.URL, APIMode: "chat", Model: "deepseek-chat"},
		TranslateProviderCfg: config.ProviderConfig{BaseURL: ts.URL, Model: "deepseek-chat"},
		APIKey:               "k",
		Rules:                testRules(),
		MaxRetries:           0,
		Client:               llm.NewClient(10 * time.Second),
		TranslateClient:      translator.NewClient(10 * time.Second),
		Logger:               nil,
		Candidate:            1,
	})
	if err == nil || !strings.Contains(err.Error(), "cn title 校验失败") {
		t.Fatalf("expected cn title validation error, got %v", err)
	}
}
