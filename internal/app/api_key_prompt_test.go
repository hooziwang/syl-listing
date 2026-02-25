package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"syl-listing/internal/config"
)

func TestEnsureDeepSeekAPIKey_ExistingKey(t *testing.T) {
	d := t.TempDir()
	envPath := filepath.Join(d, ".env")
	if err := config.UpsertEnvVar(envPath, "DEEPSEEK_API_KEY", "existing"); err != nil {
		t.Fatal(err)
	}
	envMap, key, err := ensureDeepSeekAPIKey(&config.Paths{EnvPath: envPath}, "DEEPSEEK_API_KEY")
	if err != nil {
		t.Fatalf("expected existing key success, got %v", err)
	}
	if key != "existing" || envMap["DEEPSEEK_API_KEY"] != "existing" {
		t.Fatalf("unexpected key/map: key=%q map=%v", key, envMap)
	}
}

func TestEnsureDeepSeekAPIKey_MissingEnvFile(t *testing.T) {
	d := t.TempDir()
	envPath := filepath.Join(d, ".env")
	_, _, err := ensureDeepSeekAPIKey(&config.Paths{EnvPath: envPath}, "DEEPSEEK_API_KEY")
	if err == nil || !strings.Contains(err.Error(), "尚未配置 API KEY") || !strings.Contains(err.Error(), "syl-listing set key <api_key>") {
		t.Fatalf("expected missing env hint, got %v", err)
	}
}

func TestEnsureDeepSeekAPIKey_MissingKeyVar(t *testing.T) {
	d := t.TempDir()
	envPath := filepath.Join(d, ".env")
	if err := os.WriteFile(envPath, []byte("OTHER=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := ensureDeepSeekAPIKey(&config.Paths{EnvPath: envPath}, "DEEPSEEK_API_KEY")
	if err == nil || !strings.Contains(err.Error(), "尚未配置 API KEY") {
		t.Fatalf("expected missing key hint, got %v", err)
	}
}
