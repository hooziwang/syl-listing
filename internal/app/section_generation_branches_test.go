package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
)

func TestValidateBulletLineAndBoundsBranches(t *testing.T) {
	bounds := resolveCharBounds(10, 20, 2)
	if issues, _ := validateBulletLine(1, "a\nb", bounds); len(issues) == 0 {
		t.Fatalf("expected multiline issue")
	}
	if issues, _ := validateBulletLine(2, "   ", bounds); len(issues) == 0 {
		t.Fatalf("expected empty issue")
	}
	if issues, _ := validateBulletLine(3, strings.Repeat("x", 30), bounds); len(issues) == 0 {
		t.Fatalf("expected out-of-tolerance issue")
	}
	if issues, warns := validateBulletLine(4, strings.Repeat("x", 21), bounds); len(issues) != 0 || len(warns) == 0 {
		t.Fatalf("expected tolerance warning only, issues=%v warns=%v", issues, warns)
	}
	if issues, warns := validateBulletLine(5, "ok", resolveCharBounds(0, 0, 0)); len(issues) != 0 || len(warns) != 0 {
		t.Fatalf("expected no rule no issue/warn, issues=%v warns=%v", issues, warns)
	}

	swapped := resolveCharBounds(30, 10, -5)
	if swapped.ruleMin != 10 || swapped.ruleMax != 30 || swapped.tolMin != 10 || swapped.tolMax != 30 {
		t.Fatalf("expected swapped + no negative tolerance, got %+v", swapped)
	}
	maxOnly := resolveCharBounds(0, 10, 5)
	if maxOnly.hasMin || !maxOnly.hasMax || maxOnly.tolMax != 15 {
		t.Fatalf("unexpected maxOnly bounds: %+v", maxOnly)
	}
	minOnly := resolveCharBounds(10, 0, 5)
	if !minOnly.hasMin || minOnly.hasMax || minOnly.tolMin != 5 {
		t.Fatalf("unexpected minOnly bounds: %+v", minOnly)
	}
	none := resolveCharBounds(0, 0, 10)
	if none.hasRule() || formatRangeText(0, 0, false, false) != "(-inf,+inf)" || formatRangeText(3, 0, true, false) != "[3,+inf)" {
		t.Fatalf("unexpected none bounds/range formatting: %+v", none)
	}
}

func TestValidateSectionTextExtraBranches(t *testing.T) {
	req := listing.Requirement{Category: "Cat", Keywords: []string{"alpha", "beta"}}
	titleRule := config.SectionRuleFile{
		Parsed: config.SectionRule{
			Output: config.RuleOutputSpec{Lines: 1},
			Constraints: config.RuleConstraints{
				MaxChars:                config.RuleIntConstraint{Value: 5},
				MustContainTopNKeywords: config.RuleKeywordConstraint{Value: 1},
			},
		},
	}
	if issues, _ := validateSectionText("title", "en", req, "", titleRule, 0); len(issues) == 0 {
		t.Fatalf("expected title empty issue")
	}
	if issues, _ := validateSectionText("title", "en", req, "alpha-lorem-very-long", titleRule, 1); len(issues) == 0 {
		t.Fatalf("expected title max out-of-tolerance issue")
	}
	if issues, warns := validateSectionText("title", "en", req, "alpha1", titleRule, 1); len(issues) != 0 || len(warns) == 0 {
		t.Fatalf("expected title tolerance warning, issues=%v warns=%v", issues, warns)
	}

	descRule := config.SectionRuleFile{Parsed: config.SectionRule{Output: config.RuleOutputSpec{Paragraphs: 2}}}
	if issues, _ := validateSectionText("description", "en", req, "one paragraph", descRule, 0); len(issues) == 0 {
		t.Fatalf("expected description paragraph count error")
	}

	searchRule := config.SectionRuleFile{
		Parsed: config.SectionRule{
			Output:      config.RuleOutputSpec{Lines: 1},
			Constraints: config.RuleConstraints{MaxChars: config.RuleIntConstraint{Value: 5}},
		},
	}
	if issues, _ := validateSectionText("search_terms", "en", req, " ", searchRule, 0); len(issues) == 0 {
		t.Fatalf("expected search empty issue")
	}
	if issues, _ := validateSectionText("search_terms", "en", req, "a\nb", searchRule, 0); len(issues) == 0 {
		t.Fatalf("expected search line count issue")
	}
	if issues, warns := validateSectionText("search_terms", "en", req, "abcdef", searchRule, 1); len(issues) != 0 || len(warns) == 0 {
		t.Fatalf("expected search tolerance warning, issues=%v warns=%v", issues, warns)
	}
	if issues, _ := validateSectionText("search_terms", "en", req, "abcdefg", searchRule, 1); len(issues) == 0 {
		t.Fatalf("expected search out-of-tolerance issue")
	}
}

func TestBuildRepairTrimPadAndValidateDocBranches(t *testing.T) {
	if !strings.Contains(buildSectionRepairPrompt("title", []string{"a"}), "title") {
		t.Fatalf("title repair label missing")
	}
	if !strings.Contains(buildSectionRepairPrompt("description", []string{"a"}), "product description") {
		t.Fatalf("description repair label missing")
	}
	if !strings.Contains(buildSectionRepairPrompt("search_terms", []string{"a"}), "search terms") {
		t.Fatalf("search_terms repair label missing")
	}
	if !strings.Contains(buildSectionRepairPrompt("unknown", []string{"a"}), "unknown") {
		t.Fatalf("unknown repair label missing")
	}

	if got := trimToMaxByWords("abc", 0); got != "abc" {
		t.Fatalf("max<=0 should return original: %q", got)
	}
	if got := trimToMaxByWords("superlongword", 5); got != "super" {
		t.Fatalf("single long word should fallback to rune cut: %q", got)
	}

	if got := padToMinByKeywords("base", 0, 20, []string{"k1"}); got != "base" {
		t.Fatalf("min<=0 should return original: %q", got)
	}
	if got := padToMinByKeywords("already long enough", 5, 20, []string{"k1"}); got != "already long enough" {
		t.Fatalf("already enough should not change: %q", got)
	}
	if got := padToMinByKeywords("alpha", 20, 25, []string{"alpha"}); !strings.Contains(got, "for ") {
		t.Fatalf("should hit fallback extension, got: %q", got)
	}
	if got := padToMinByKeywords("alpha", 20, 6, []string{"beta"}); got != "alpha" {
		t.Fatalf("max too small should block extension: %q", got)
	}

	rules := testRules()
	req := listing.Requirement{Category: "Cat", Keywords: []string{"k1", "k2"}}
	baseDoc := ListingDocument{
		Category:              "Cat",
		Keywords:              []string{"k1", "k2"},
		BulletPoints:          []string{"1", "2", "3", "4", "5"},
		DescriptionParagraphs: []string{"p1", "p2"},
		SearchTerms:           "x",
	}
	doc := baseDoc
	doc.Category = "Other"
	if err := validateDocumentBySectionRules("en", req, doc, rules); err == nil || !strings.Contains(err.Error(), "category 与输入不一致") {
		t.Fatalf("expected category mismatch error, got %v", err)
	}
	doc = baseDoc
	doc.Keywords = nil
	if err := validateDocumentBySectionRules("en", req, doc, rules); err == nil || !strings.Contains(err.Error(), "keywords 为空") {
		t.Fatalf("expected keywords empty error, got %v", err)
	}
	doc = baseDoc
	doc.Keywords = []string{"k1"}
	if err := validateDocumentBySectionRules("en", req, doc, rules); err == nil || !strings.Contains(err.Error(), "keywords 数量与输入不一致") {
		t.Fatalf("expected en keyword count error, got %v", err)
	}
	doc = baseDoc
	doc.Keywords = []string{"k1"}
	if err := validateDocumentBySectionRules("cn", req, doc, rules); err == nil || !strings.Contains(err.Error(), "cn keywords 数量错误") {
		t.Fatalf("expected cn keyword count error, got %v", err)
	}
	doc = baseDoc
	doc.Keywords = []string{"k1", " "}
	if err := validateDocumentBySectionRules("cn", req, doc, rules); err == nil || !strings.Contains(err.Error(), "cn keywords 第2项为空") {
		t.Fatalf("expected cn keyword empty item error, got %v", err)
	}
	doc = baseDoc
	doc.BulletPoints = []string{"1"}
	if err := validateDocumentBySectionRules("en", req, doc, rules); err == nil || !strings.Contains(err.Error(), "五点数量错误") {
		t.Fatalf("expected bullet count error, got %v", err)
	}
	doc = baseDoc
	doc.DescriptionParagraphs = []string{"p1"}
	if err := validateDocumentBySectionRules("en", req, doc, rules); err == nil || !strings.Contains(err.Error(), "描述段落数量错误") {
		t.Fatalf("expected description count error, got %v", err)
	}
}

func TestGenerateSectionAndBulletsExtraBranches(t *testing.T) {
	opts := sectionGenerateOptions{
		Req:           listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "x", Category: "Cat", Keywords: []string{"a"}},
		Lang:          "en",
		Provider:      "deepseek",
		ProviderCfg:   config.ProviderConfig{BaseURL: "http://127.0.0.1:1", APIMode: "chat", Model: "deepseek-chat"},
		APIKey:        "k",
		Rules:         testRules(),
		MaxRetries:    0,
		Client:        llm.NewClient(50 * time.Millisecond),
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}
	if _, _, err := generateSectionWithRetry(opts, "unknown", ListingDocument{}); err == nil || !strings.Contains(err.Error(), "未知分段") {
		t.Fatalf("expected unknown step error, got %v", err)
	}

	rule := config.SectionRuleFile{Parsed: config.SectionRule{Output: config.RuleOutputSpec{Lines: 0}}}
	if _, _, err := generateBulletsWithJSONAndRepair(opts, ListingDocument{}, rule); err == nil || !strings.Contains(err.Error(), "output.lines 无效") {
		t.Fatalf("expected invalid bullets rule error, got %v", err)
	}

	// force path where validateBulletSet only returns warnings and no invalid indexes
	items := []string{
		strings.Repeat("x", 9),
		strings.Repeat("x", 9),
		strings.Repeat("x", 9),
		strings.Repeat("x", 9),
		strings.Repeat("x", 9),
	}
	invalid, issues, warns := validateBulletSet(items, resolveCharBounds(10, 40, 2))
	if len(invalid) != 0 || len(issues) != 0 || len(warns) == 0 {
		t.Fatalf("expected warnings only, invalid=%v issues=%v warns=%v", invalid, issues, warns)
	}
}

func TestRegenerateBulletItemThinkingFallbackBranch(t *testing.T) {
	var seenReasoner bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/chat/completions") {
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), `"model":"deepseek-reasoner"`) {
				seenReasoner = true
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	opts := sectionGenerateOptions{
		Req:      listing.Requirement{SourcePath: "/tmp/a.md", BodyAfterMarker: "x", Category: "Cat", Keywords: []string{"a"}},
		Lang:     "en",
		Provider: "deepseek",
		ProviderCfg: config.ProviderConfig{
			BaseURL: ts.URL,
			APIMode: "chat",
			Model:   "deepseek-chat",
			ThinkingFallback: config.ThinkingFallbackConfig{
				Enabled: true,
				Attempt: 2,
				Model:   "deepseek-reasoner",
			},
		},
		APIKey:        "k",
		Rules:         testRules(),
		MaxRetries:    1,
		Client:        llm.NewClient(2 * time.Second),
		Logger:        nil,
		Candidate:     1,
		CharTolerance: 0,
	}
	_, _, err := regenerateBulletItemJSONWithRetry(
		opts,
		ListingDocument{Title: "t"},
		testRules().Bullets,
		1,
		[]string{"a", "b", "c", "d", "e"},
		resolveCharBounds(10, 40, 0),
	)
	if err == nil || !strings.Contains(err.Error(), "重试后仍失败") {
		t.Fatalf("expected retry failure, got %v", err)
	}
	if !seenReasoner {
		t.Fatalf("expected thinking fallback model request")
	}
}
