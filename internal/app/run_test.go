package app

import (
	"path/filepath"
	"testing"

	"syl-listing/internal/config"
)

func TestOverrideConfigAndAbsPath(t *testing.T) {
	cfg := &config.Config{Output: config.OutputConfig{Dir: ".", Num: 1}}
	overrideConfig(cfg, Options{OutputDir: "./out", Num: 3, Concurrency: 9, MaxRetries: 2, Provider: "deepseek"})
	if cfg.Output.Dir != "./out" || cfg.Output.Num != 3 || cfg.Concurrency != 9 || cfg.MaxRetries != 2 {
		t.Fatalf("override mismatch: %+v", cfg)
	}
	cwd := "/tmp/x"
	if got := absPath(cwd, "a/b"); got != filepath.Join(cwd, "a/b") {
		t.Fatalf("absPath mismatch: %s", got)
	}
}
