package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"syl-listing/internal/listing"
)

func TestDiscoverFilesAndDir(t *testing.T) {
	d := t.TempDir()
	good1 := filepath.Join(d, "a.md")
	good2 := filepath.Join(d, "sub", "b.md")
	bad := filepath.Join(d, "bad.md")
	if err := os.MkdirAll(filepath.Dir(good2), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good1, []byte(listing.Marker+"\n品牌名: A\n分类: C\n# 关键词库\n- x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good2, []byte(listing.Marker+"\n品牌名: B\n分类: C\n# 关键词库\n- y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover([]string{good1, d})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("files len mismatch: %d, %+v", len(res.Files), res.Files)
	}
}

func TestDiscoverInvalidInput(t *testing.T) {
	_, err := Discover([]string{"/not/exist"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestDiscoverInvalidSingleFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "x.md")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Discover([]string{p})
	if err == nil || !strings.Contains(err.Error(), "缺少首行标志") {
		t.Fatalf("unexpected err: %v", err)
	}
}
