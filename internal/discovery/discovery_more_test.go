package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"syl-listing/internal/listing"
)

func TestDiscoverEmptyAndBlankInputs(t *testing.T) {
	if _, err := Discover(nil); err == nil || !strings.Contains(err.Error(), "未提供输入路径") {
		t.Fatalf("expected empty input error, got %v", err)
	}
	if _, err := Discover([]string{"  "}); err == nil || !strings.Contains(err.Error(), "未找到任何可用") {
		t.Fatalf("expected blank input error, got %v", err)
	}
}

func TestDiscoverDirWithoutListingFiles(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "a.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover([]string{d}); err == nil || !strings.Contains(err.Error(), "未找到任何可用") {
		t.Fatalf("expected no listing files error, got %v", err)
	}
}

func TestDiscoverSkipsHiddenDirs(t *testing.T) {
	d := t.TempDir()
	hiddenDir := filepath.Join(d, ".hidden")
	visibleDir := filepath.Join(d, "visible")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(visibleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hiddenReq := filepath.Join(hiddenDir, "h.md")
	visibleReq := filepath.Join(visibleDir, "v.md")
	content := []byte(listing.Marker + "\n品牌名: A\n分类: C\n# 关键词库\n- x")
	if err := os.WriteFile(hiddenReq, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(visibleReq, content, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Discover([]string{d})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(res.Files) != 1 || res.Files[0] != visibleReq {
		t.Fatalf("expected only visible file, got %+v", res.Files)
	}
}
