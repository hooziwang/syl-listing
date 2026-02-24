package app

import (
	"strings"
	"testing"

	"syl-listing/internal/listing"
)

func TestRenderMarkdownENAndCN(t *testing.T) {
	req := listing.Requirement{Brand: "BrandX"}
	doc := ListingDocument{
		Title:                 "My Title",
		Keywords:              []string{"k1", "k2"},
		Category:              "Cat",
		BulletPoints:          []string{"b1", "b2", "b3", "b4", "b5"},
		DescriptionParagraphs: []string{"p1", "p2"},
		SearchTerms:           "s1 s2",
	}
	en := RenderMarkdown("en", req, doc)
	cn := RenderMarkdown("cn", req, doc)
	if !strings.Contains(en, "## Bullet Points") || !strings.Contains(en, "**Point 1**") {
		t.Fatalf("invalid en markdown: %s", en)
	}
	if !strings.Contains(cn, "## 五点描述") || !strings.Contains(cn, "**第1点**") {
		t.Fatalf("invalid cn markdown: %s", cn)
	}
}

func TestRuneLenAndDedupeIssues(t *testing.T) {
	if runeLen("你好a") != 3 {
		t.Fatalf("runeLen mismatch")
	}
	out := dedupeIssues([]string{" a ", "a", "", "b"})
	if len(out) != 2 || out[0] != "a" || out[1] != "b" {
		t.Fatalf("dedupeIssues: %#v", out)
	}
}
