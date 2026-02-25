package app

import (
	"encoding/json"
	"strings"
	"testing"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
)

func boolPtrApp(v bool) *bool { return &v }

func testRules() config.SectionRules {
	disableThinking := true
	return config.SectionRules{
		Title: config.SectionRuleFile{Raw: " title rule ", Parsed: config.SectionRule{
			Output:      config.RuleOutputSpec{Lines: 1},
			Constraints: config.RuleConstraints{MaxChars: config.RuleIntConstraint{Value: 200}, MustContainTopNKeywords: config.RuleKeywordConstraint{Value: 2}},
			Execution: config.RuleExecutionSpec{
				Generation: config.RuleGenerationSpec{Protocol: "text"},
				Repair:     config.RuleRepairPolicySpec{Granularity: "whole"},
				Fallback:   config.RuleFallbackPolicySpec{DisableThinkingOnLengthError: &disableThinking},
			},
		}},
		Bullets: config.SectionRuleFile{Parsed: config.SectionRule{
			Output:      config.RuleOutputSpec{Format: "json_object", Lines: 5},
			Constraints: config.RuleConstraints{MinCharsPerLine: config.RuleIntConstraint{Value: 10}, MaxCharsPerLine: config.RuleIntConstraint{Value: 40}},
			Execution: config.RuleExecutionSpec{
				Generation: config.RuleGenerationSpec{Protocol: "json_lines"},
				Repair:     config.RuleRepairPolicySpec{Granularity: "item", ItemJSONField: "item"},
				Fallback:   config.RuleFallbackPolicySpec{DisableThinkingOnLengthError: &disableThinking},
			},
		}},
		Description: config.SectionRuleFile{Parsed: config.SectionRule{
			Output: config.RuleOutputSpec{Paragraphs: 2},
			Execution: config.RuleExecutionSpec{
				Generation: config.RuleGenerationSpec{Protocol: "text"},
				Repair:     config.RuleRepairPolicySpec{Granularity: "whole"},
				Fallback:   config.RuleFallbackPolicySpec{DisableThinkingOnLengthError: &disableThinking},
			},
		}},
		SearchTerms: config.SectionRuleFile{Parsed: config.SectionRule{
			Output:      config.RuleOutputSpec{Lines: 1},
			Constraints: config.RuleConstraints{MaxChars: config.RuleIntConstraint{Value: 50}},
			Execution: config.RuleExecutionSpec{
				Generation: config.RuleGenerationSpec{Protocol: "text"},
				Repair:     config.RuleRepairPolicySpec{Granularity: "whole"},
				Fallback:   config.RuleFallbackPolicySpec{DisableThinkingOnLengthError: &disableThinking},
			},
		}},
	}
}

func TestParseBulletsHelpers(t *testing.T) {
	items, err := parseBullets("1) a\n2) b\n3) c\n4) d\n5) e", 5)
	if err != nil || len(items) != 5 {
		t.Fatalf("parseBullets failed: %v %+v", err, items)
	}
	inline := splitInlineBullets("1) a 2) b 3) c 4) d 5) e")
	if len(inline) != 5 {
		t.Fatalf("splitInlineBullets failed: %+v", inline)
	}
	items, err = parseBullets("a; b; c; d; e", 5)
	if err != nil || len(items) != 5 {
		t.Fatalf("parse semi bullets failed: %v %+v", err, items)
	}
	if _, err := parseBullets("a\nb", 5); err == nil {
		t.Fatalf("expected bullet count error")
	}
}

func TestJSONParsers(t *testing.T) {
	b, err := parseBulletsFromJSON(`{"bullets":["a","b","c","d","e"]}`, 5)
	if err != nil || len(b) != 5 {
		t.Fatalf("parseBulletsFromJSON failed: %v %+v", err, b)
	}
	b, err = parseBulletsFromJSON("prefix {\"items\":[\"a\",\"b\",\"c\",\"d\",\"e\"]} suffix", 5)
	if err != nil || len(b) != 5 {
		t.Fatalf("parse items json failed: %v %+v", err, b)
	}
	if _, err := parseBulletsFromJSON(`{"bullets":["a"]}`, 5); err == nil {
		t.Fatalf("expected count error")
	}
	line, err := parseBulletItemFromJSON(`{"text":"abc"}`)
	if err != nil || line != "abc" {
		t.Fatalf("parseBulletItemFromJSON failed: %v, %s", err, line)
	}
	if line, err := parseBulletItemFromJSON(`{"x":"y"}`); err != nil || line != "y" {
		t.Fatalf("expected fallback first-string parse, err=%v line=%q", err, line)
	}
	if err := decodeJSONObject("", &map[string]any{}); err == nil {
		t.Fatalf("expected empty json error")
	}
	if got := extractJSONObject("a {\"x\":1} b"); got == "" {
		t.Fatalf("expected object")
	}
}

func TestPromptBuilders(t *testing.T) {
	req := listing.Requirement{BodyAfterMarker: "需求", Category: "Cat", Keywords: []string{"k1", "k2"}}
	doc := ListingDocument{Title: "t", BulletPoints: []string{"b1"}, DescriptionParagraphs: []string{"p1"}}
	if got := buildSectionSystemPrompt(config.SectionRuleFile{Raw: "  x  "}); got != "x" {
		t.Fatalf("unexpected system prompt: %q", got)
	}
	up := buildSectionUserPrompt("title", req, doc)
	if !strings.Contains(up, "【当前任务】生成：title") || !strings.Contains(up, "k1") {
		t.Fatalf("unexpected user prompt: %s", up)
	}
	rp := buildSectionRepairPrompt("bullets", []string{"a", "b"})
	if !strings.Contains(rp, "bullets") {
		t.Fatalf("unexpected repair prompt: %s", rp)
	}
	jp := buildJSONRepairPrompt("bad", `{"x":1}`)
	if !strings.Contains(jp, "JSON") {
		t.Fatalf("unexpected json repair prompt: %s", jp)
	}
}

func TestResolveModelForAttempt(t *testing.T) {
	opts := sectionGenerateOptions{
		Provider: "deepseek",
		ProviderCfg: config.ProviderConfig{
			Model:            "deepseek-chat",
			ThinkingFallback: config.ThinkingFallbackConfig{Enabled: true, Attempt: 3, Model: "deepseek-reasoner"},
		},
	}
	rule := testRules().Bullets
	if m, fb := resolveModelForAttempt(opts, rule, 2, false); m != "deepseek-chat" || fb {
		t.Fatalf("unexpected before fallback: %s %v", m, fb)
	}
	if m, fb := resolveModelForAttempt(opts, rule, 3, false); m != "deepseek-reasoner" || !fb {
		t.Fatalf("unexpected fallback: %s %v", m, fb)
	}
	if m, fb := resolveModelForAttempt(opts, rule, 3, true); m != "deepseek-chat" || fb {
		t.Fatalf("length issue should disable thinking fallback: %s %v", m, fb)
	}
	opts.Provider = "openai"
	if m, fb := resolveModelForAttempt(opts, rule, 3, false); m == "" || fb {
		t.Fatalf("unexpected non-deepseek fallback")
	}
}

func TestValidateSectionText(t *testing.T) {
	rules := testRules()
	req := listing.Requirement{Category: "Cat", Keywords: []string{"alpha", "beta"}}

	issues, warns := validateSectionText("title", "en", req, "alpha beta title", rules.Title, 20)
	if len(issues) != 0 || len(warns) != 0 {
		t.Fatalf("title should pass: issues=%v warns=%v", issues, warns)
	}
	issues, _ = validateSectionText("title", "en", req, "alpha only", rules.Title, 20)
	if len(issues) == 0 {
		t.Fatalf("expected title keyword issue")
	}

	issues, warns = validateSectionText("bullets", "en", req, "a\nb\nc\nd\ne", rules.Bullets, 0)
	if len(issues) == 0 {
		t.Fatalf("expected bullets too short issues")
	}
	longEnough := strings.Repeat("x", 9)
	text := strings.Join([]string{longEnough, longEnough, longEnough, longEnough, longEnough}, "\n")
	issues, warns = validateSectionText("bullets", "en", req, text, rules.Bullets, 20)
	if len(issues) != 0 || len(warns) == 0 {
		t.Fatalf("expected tolerance warning only, issues=%v warns=%v", issues, warns)
	}

	issues, _ = validateSectionText("description", "en", req, "p1\n\np2", rules.Description, 20)
	if len(issues) != 0 {
		t.Fatalf("description should pass: %v", issues)
	}
	issues, _ = validateSectionText("search_terms", "en", req, "a b c", rules.SearchTerms, 20)
	if len(issues) != 0 {
		t.Fatalf("search_terms should pass: %v", issues)
	}
}

func TestCharBoundsAndFormatting(t *testing.T) {
	b := resolveCharBounds(10, 20, 3)
	if !b.hasRule() || !b.inRule(15) || b.inRule(25) || !b.inTolerance(22) {
		t.Fatalf("bounds mismatch: %+v", b)
	}
	if b.ruleText() != "[10,20]" || b.toleranceText() != "[7,23]" {
		t.Fatalf("range text mismatch: %s %s", b.ruleText(), b.toleranceText())
	}
	if got := formatRangeText(0, 20, false, true); got != "(-inf,20]" {
		t.Fatalf("format range mismatch: %s", got)
	}
}

func TestTextNormalizeCleanAndSplit(t *testing.T) {
	if got := normalizeModelText("```text\\na\\n```"); !strings.Contains(got, "a") {
		t.Fatalf("normalizeModelText mismatch: %q", got)
	}
	if got := normalizeSingleLine("\n x \n y"); got != "x" {
		t.Fatalf("normalizeSingleLine mismatch: %q", got)
	}
	if cleanTitleLine("标题: - hello") != "- hello" {
		t.Fatalf("cleanTitleLine mismatch")
	}
	if cleanSearchTermsLine("Search Terms: x y") != "x y" {
		t.Fatalf("cleanSearchTermsLine mismatch")
	}
	if cleanBulletLine("1) - x") != "- x" {
		t.Fatalf("cleanBulletLine mismatch")
	}
	if cleanCategoryLine("分类: A>B") != "A>B" {
		t.Fatalf("cleanCategoryLine mismatch")
	}
	if cleanKeywordLine("关键词: - aa") != "aa" {
		t.Fatalf("cleanKeywordLine mismatch")
	}

	p, err := parseParagraphs("a\n\n b", 2)
	if err != nil || len(p) != 2 {
		t.Fatalf("parseParagraphs failed: %v %+v", err, p)
	}
	if countNonEmptyLines("a\n\n b") != 2 {
		t.Fatalf("countNonEmptyLines mismatch")
	}
	if len(splitByBlankLines("a\n\n b")) != 2 {
		t.Fatalf("splitByBlankLines mismatch")
	}
}

func TestTrimAndPadHelpers(t *testing.T) {
	if got := trimToMaxByWords("a bb ccc", 4); got != "a bb" {
		t.Fatalf("trimToMaxByWords mismatch: %q", got)
	}
	if got := trimToMaxByWords("abcdef", 3); got != "abc" {
		t.Fatalf("trimToMaxByWords fallback mismatch: %q", got)
	}
	padded := padToMinByKeywords("base", 12, 40, []string{"k1", "k2"})
	if runeLen(padded) < 12 {
		t.Fatalf("padToMinByKeywords too short: %q", padded)
	}
}

func TestValidateDocumentBySectionRules(t *testing.T) {
	rules := testRules()
	req := listing.Requirement{Category: "Cat", Keywords: []string{"k1", "k2"}}
	doc := ListingDocument{
		Category:              "Cat",
		Keywords:              []string{"k1", "k2"},
		BulletPoints:          []string{"1", "2", "3", "4", "5"},
		DescriptionParagraphs: []string{"p1", "p2"},
		SearchTerms:           "x",
	}
	if err := validateDocumentBySectionRules("en", req, doc, rules); err != nil {
		t.Fatalf("en validation should pass: %v", err)
	}
	docCN := doc
	docCN.Category = "分类"
	docCN.Keywords = []string{"词1", "词2"}
	if err := validateDocumentBySectionRules("cn", req, docCN, rules); err != nil {
		t.Fatalf("cn validation should pass: %v", err)
	}
	docBad := doc
	docBad.Category = ""
	if err := validateDocumentBySectionRules("en", req, docBad, rules); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseBulletJSONFromEncodedResponse(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"bullets": []string{"a", "b", "c", "d", "e"}})
	items, err := parseBulletsFromJSON(string(payload), 5)
	if err != nil || len(items) != 5 {
		t.Fatalf("unexpected parse result: %v %v", items, err)
	}
}
