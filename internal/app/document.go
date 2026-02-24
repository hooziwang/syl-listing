package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"syl-listing/internal/listing"
)

type ListingDocument struct {
	Title                 string
	Keywords              []string
	Category              string
	BulletPoints          []string
	DescriptionParagraphs []string
	SearchTerms           string
}

func RenderMarkdown(lang string, req listing.Requirement, doc ListingDocument) string {
	var b strings.Builder
	if lang == "en" {
		b.WriteString("# ")
		b.WriteString(strings.TrimSpace(req.Brand))
		b.WriteString(" Listing\n\n")
		b.WriteString("## Keywords\n")
		for _, kw := range doc.Keywords {
			b.WriteString(kw)
			b.WriteString("\n")
		}
		b.WriteString("\n## Category\n")
		b.WriteString(doc.Category)
		b.WriteString("\n\n## Title\n")
		b.WriteString(doc.Title)
		b.WriteString("\n\n## Bullet Points\n")
		for i, bp := range doc.BulletPoints {
			b.WriteString(fmt.Sprintf("**Point %d**\n", i+1))
			b.WriteString(bp)
			b.WriteString("\n\n")
		}
		b.WriteString("## Product Description\n")
		for i, p := range doc.DescriptionParagraphs {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p)
			b.WriteString("\n")
		}
		b.WriteString("\n## Search Terms\n")
		b.WriteString(doc.SearchTerms)
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString("# ")
	b.WriteString(strings.TrimSpace(req.Brand))
	b.WriteString(" 产品Listing\n\n")
	b.WriteString("## 关键词\n")
	for _, kw := range doc.Keywords {
		b.WriteString(kw)
		b.WriteString("\n")
	}
	b.WriteString("\n## 分类\n")
	b.WriteString(doc.Category)
	b.WriteString("\n\n## 标题\n")
	b.WriteString(doc.Title)
	b.WriteString("\n\n## 五点描述\n")
	for i, bp := range doc.BulletPoints {
		b.WriteString(fmt.Sprintf("**第%d点**\n", i+1))
		b.WriteString(bp)
		b.WriteString("\n\n")
	}
	b.WriteString("## 产品描述\n")
	for i, p := range doc.DescriptionParagraphs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p)
		b.WriteString("\n")
	}
	b.WriteString("\n## 搜索词\n")
	b.WriteString(doc.SearchTerms)
	b.WriteString("\n")
	return b.String()
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func dedupeIssues(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, it := range in {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}
