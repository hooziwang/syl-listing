package config

import (
	"strings"
	"testing"
)

func TestSectionRulesGet(t *testing.T) {
	r := SectionRules{
		Title:       SectionRuleFile{Parsed: SectionRule{Output: RuleOutputSpec{Lines: 1}}},
		Bullets:     SectionRuleFile{Parsed: SectionRule{Output: RuleOutputSpec{Lines: 5}}},
		Description: SectionRuleFile{Parsed: SectionRule{Output: RuleOutputSpec{Paragraphs: 2}}},
		SearchTerms: SectionRuleFile{},
	}
	if _, err := r.Get("title"); err != nil {
		t.Fatalf("title err: %v", err)
	}
	if _, err := r.Get("bullets"); err != nil {
		t.Fatalf("bullets err: %v", err)
	}
	if _, err := r.Get("description"); err != nil {
		t.Fatalf("description err: %v", err)
	}
	if _, err := r.Get("search_terms"); err != nil {
		t.Fatalf("search_terms err: %v", err)
	}
	if _, err := r.Get("unknown"); err == nil || !strings.Contains(err.Error(), "未知分段") {
		t.Fatalf("expected unknown step error, got %v", err)
	}
	if r.BulletCount() != 5 || r.DescriptionParagraphs() != 2 {
		t.Fatalf("counts mismatch")
	}
}
