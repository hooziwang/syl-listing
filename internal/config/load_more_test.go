package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigFormatError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	cfgPath := filepath.Join(home, ".syl-listing", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("provider: [deepseek"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(cfgPath, home)
	if err == nil || !strings.Contains(err.Error(), "配置文件格式错误") {
		t.Fatalf("expected config format error, got %v", err)
	}
}

func TestEnsureBootstrapMkdirError(t *testing.T) {
	d := t.TempDir()
	blocker := filepath.Join(d, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ensureBootstrap(&Paths{
		ConfigPath: filepath.Join(blocker, "config.yaml"),
		EnvExample: filepath.Join(d, "x", ".env.example"),
		RulesDir:   filepath.Join(d, "rules"),
	})
	if err == nil || !strings.Contains(err.Error(), "创建配置目录失败") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestEnsureFilePathIsDirectory(t *testing.T) {
	d := t.TempDir()
	targetDir := filepath.Join(d, "as-dir")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureFile(targetDir, []byte("x"), 0o644); err == nil {
		t.Fatalf("expected ensureFile error when path is directory")
	}
}

func TestReadSectionRulesReadFailureNonNotExist(t *testing.T) {
	d := t.TempDir()
	writeRuleFiles(t, d)
	// make one rule path a directory so os.ReadFile returns non-not-exist error.
	badPath := filepath.Join(d, "title.yaml")
	if err := os.Remove(badPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(badPath, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ReadSectionRules(d)
	if err == nil || !strings.Contains(err.Error(), "读取规则文件失败") {
		t.Fatalf("expected read failure error, got %v", err)
	}
}
