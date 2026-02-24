package listing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBodyAfterMarker(t *testing.T) {
	t.Run("valid with bom", func(t *testing.T) {
		raw := "\ufeff\n" + Marker + "\n品牌名: A\n分类: C"
		body, ok := BodyAfterMarker(raw)
		if !ok {
			t.Fatalf("expected ok")
		}
		if !strings.Contains(body, "品牌名: A") {
			t.Fatalf("unexpected body: %q", body)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, ok := BodyAfterMarker("hello")
		if ok {
			t.Fatalf("expected invalid")
		}
	})
}

func TestIsListingRequirements(t *testing.T) {
	if !IsListingRequirements(Marker + "\nbody") {
		t.Fatalf("expected true")
	}
	if IsListingRequirements("hello") {
		t.Fatalf("expected false")
	}
}

func TestParseFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "req.md")
	content := strings.Join([]string{
		Marker,
		"品牌名: DemoBrand",
		"分类: Home & Kitchen",
		"# 关键词库",
		"1. alpha",
		"- beta",
		"* gamma",
		"• delta",
		"5) epsilon",
		"# 其它",
	}, "\n")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	req, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if req.Brand != "DemoBrand" {
		t.Fatalf("brand mismatch: %q", req.Brand)
	}
	if req.Category != "Home & Kitchen" {
		t.Fatalf("category mismatch: %q", req.Category)
	}
	if len(req.Keywords) != 5 {
		t.Fatalf("keywords len mismatch: %d", len(req.Keywords))
	}
	if len(req.Warnings) == 0 {
		t.Fatalf("expected keyword count warning")
	}
}

func TestParseFileInvalidMarker(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "bad.md")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseFile(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "缺少首行标志") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestParseCategoryFromSectionHeader(t *testing.T) {
	body := "# 分类\nTools > Light\n# 关键词库\n- a"
	if got := parseCategory(body); got != "Tools > Light" {
		t.Fatalf("parseCategory got %q", got)
	}
}
