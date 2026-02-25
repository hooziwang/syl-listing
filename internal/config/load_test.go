package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func writeRuleFiles(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"title.yaml": `version: 1
section: title
language: en
purpose: t
output:
  format: plain_text
  lines: 1
constraints:
  max_chars:
    value: 200
forbidden: []
execution:
  generation:
    protocol: text
  repair:
    granularity: whole
  fallback:
    disable_thinking_on_length_error: true
instruction: x
`,
		"bullets.yaml": `version: 1
section: bullets
language: en
purpose: b
output:
  format: json_object
  lines: 5
constraints:
  min_chars_per_line:
    value: 10
  max_chars_per_line:
    value: 20
forbidden: []
execution:
  generation:
    protocol: json_lines
  repair:
    granularity: item
    item_json_field: item
  fallback:
    disable_thinking_on_length_error: true
instruction: x
`,
		"description.yaml": `version: 1
section: description
language: en
purpose: d
output:
  format: plain_text
  paragraphs: 2
constraints: {}
forbidden: []
execution:
  generation:
    protocol: text
  repair:
    granularity: whole
  fallback:
    disable_thinking_on_length_error: true
instruction: x
`,
		"search_terms.yaml": `version: 1
section: search_terms
language: en
purpose: s
output:
  format: plain_text
  lines: 1
constraints:
  max_chars:
    value: 100
forbidden: []
execution:
  generation:
    protocol: text
  repair:
    granularity: whole
  fallback:
    disable_thinking_on_length_error: true
instruction: x
`,
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadAndReadRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	cfgPath := filepath.Join(home, ".syl-listing", "config.yaml")
	cfgDir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("provider: deepseek\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, paths, err := Load(cfgPath, home)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Provider != "deepseek" {
		t.Fatalf("provider: %s", cfg.Provider)
	}
	if strings.TrimSpace(paths.ResolvedRulesDir) == "" {
		t.Fatalf("resolved rules dir empty")
	}
	if !strings.Contains(paths.ResolvedRulesDir, "syl-listing") {
		t.Fatalf("unexpected rules dir: %s", paths.ResolvedRulesDir)
	}
	if runtime.GOOS != "windows" && strings.Contains(paths.ResolvedRulesDir, "~") {
		t.Fatalf("rules dir should be expanded: %s", paths.ResolvedRulesDir)
	}

	writeRuleFiles(t, paths.ResolvedRulesDir)
	rules, err := ReadSectionRules(paths.ResolvedRulesDir)
	if err != nil {
		t.Fatalf("ReadSectionRules error: %v", err)
	}
	if rules.BulletCount() != 5 {
		t.Fatalf("bullet count mismatch")
	}
}

func TestExpandPath(t *testing.T) {
	home := "/tmp/home"
	if got := expandPath("~/x", home, ""); !strings.Contains(got, "home") {
		t.Fatalf("expand ~ failed: %s", got)
	}
	if got := expandPath("a/b", home, "/cwd"); got != "/cwd/a/b" {
		t.Fatalf("expand relative failed: %s", got)
	}
}

func TestReadSectionRulesMissingAndInvalid(t *testing.T) {
	d := t.TempDir()
	if _, err := ReadSectionRules(d); err == nil {
		t.Fatalf("expected missing rules error")
	}

	writeRuleFiles(t, d)
	// break one rule to hit validate error path
	if err := os.WriteFile(filepath.Join(d, "title.yaml"), []byte("section: title\ninstruction: x\noutput:\n  lines: 2\nconstraints:\n  max_chars:\n    value: 1\nexecution:\n  generation:\n    protocol: text\n  repair:\n    granularity: whole\n  fallback:\n    disable_thinking_on_length_error: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSectionRules(d); err == nil {
		t.Fatalf("expected invalid rule error")
	}
}

func TestEnsureFileAndEnsureRuleDir(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "x.txt")
	if err := ensureFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatalf("ensureFile error: %v", err)
	}
	if err := ensureFile(p, []byte("new"), 0o644); err != nil {
		t.Fatalf("ensureFile second write should be noop: %v", err)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != "abc" {
		t.Fatalf("ensureFile should not overwrite existing file")
	}
	if err := ensureRuleDir(""); err == nil {
		t.Fatalf("expected empty rule dir error")
	}
	if err := ensureRuleDir(filepath.Join(d, "rules")); err != nil {
		t.Fatalf("ensureRuleDir error: %v", err)
	}
}

func TestValidateSectionRuleCases(t *testing.T) {
	base := SectionRule{Instruction: "x"}
	if err := validateSectionRule(base, "title.yaml", "/tmp/title.yaml"); err == nil {
		t.Fatalf("expected section mismatch error")
	}

	title := SectionRule{
		Section:     "title",
		Instruction: "x",
		Output:      RuleOutputSpec{Format: "plain_text", Lines: 1},
		Constraints: RuleConstraints{MaxChars: RuleIntConstraint{Value: 1}},
		Execution: RuleExecutionSpec{
			Generation: RuleGenerationSpec{Protocol: "text"},
			Repair:     RuleRepairPolicySpec{Granularity: "whole"},
			Fallback:   RuleFallbackPolicySpec{DisableThinkingOnLengthError: boolPtr(true)},
		},
	}
	if err := validateSectionRule(title, "title.yaml", "/tmp/title.yaml"); err != nil {
		t.Fatalf("title should pass: %v", err)
	}

	bullets := SectionRule{
		Section:     "bullets",
		Instruction: "x",
		Output:      RuleOutputSpec{Format: "json_object", Lines: 5},
		Constraints: RuleConstraints{MinCharsPerLine: RuleIntConstraint{Value: 2}, MaxCharsPerLine: RuleIntConstraint{Value: 1}},
		Execution: RuleExecutionSpec{
			Generation: RuleGenerationSpec{Protocol: "json_lines"},
			Repair:     RuleRepairPolicySpec{Granularity: "item", ItemJSONField: "item"},
			Fallback:   RuleFallbackPolicySpec{DisableThinkingOnLengthError: boolPtr(true)},
		},
	}
	if err := validateSectionRule(bullets, "bullets.yaml", "/tmp/bullets.yaml"); err == nil {
		t.Fatalf("expected min>max error")
	}

	desc := SectionRule{
		Section:     "description",
		Instruction: "x",
		Output:      RuleOutputSpec{Format: "plain_text", Paragraphs: 0},
		Execution: RuleExecutionSpec{
			Generation: RuleGenerationSpec{Protocol: "text"},
			Repair:     RuleRepairPolicySpec{Granularity: "whole"},
			Fallback:   RuleFallbackPolicySpec{DisableThinkingOnLengthError: boolPtr(true)},
		},
	}
	if err := validateSectionRule(desc, "description.yaml", "/tmp/description.yaml"); err == nil {
		t.Fatalf("expected description paragraphs error")
	}

	search := SectionRule{
		Section:     "search_terms",
		Instruction: "x",
		Output:      RuleOutputSpec{Format: "plain_text", Lines: 1},
		Constraints: RuleConstraints{MaxChars: RuleIntConstraint{Value: 10}},
		Execution: RuleExecutionSpec{
			Generation: RuleGenerationSpec{Protocol: "text"},
			Repair:     RuleRepairPolicySpec{Granularity: "whole"},
			Fallback:   RuleFallbackPolicySpec{DisableThinkingOnLengthError: boolPtr(true)},
		},
	}
	if err := validateSectionRule(search, "search_terms.yaml", "/tmp/search_terms.yaml"); err != nil {
		t.Fatalf("search_terms should pass: %v", err)
	}
}
