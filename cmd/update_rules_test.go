package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"syl-listing/internal/config"
)

func TestUpdateRulesCommandClearsCacheAndSyncLatest(t *testing.T) {
	out, err := os.Create(filepath.Join(t.TempDir(), "stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errf, err := os.Create(filepath.Join(t.TempDir(), "stderr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer errf.Close()

	work := t.TempDir()
	rulesDir := filepath.Join(work, "rules")
	lockPath := filepath.Join(work, "rules.lock")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "title.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldLoad := loadConfigForUpdate
	oldSync := syncRulesForUpdate
	defer func() {
		loadConfigForUpdate = oldLoad
		syncRulesForUpdate = oldSync
	}()

	loadConfigForUpdate = func(pathArg, cwd string) (*config.Config, *config.Paths, error) {
		return &config.Config{
				RulesCenter: config.RulesCenterConfig{
					Owner:   "hooziwang",
					Repo:    "syl-listing-rules",
					Release: "rules-v-old",
					Asset:   "rules-bundle.tar.gz",
					Strict:  false,
				},
			}, &config.Paths{
				ResolvedRulesDir: rulesDir,
				RulesLockPath:    lockPath,
			}, nil
	}
	called := false
	syncRulesForUpdate = func(cfg *config.Config, paths *config.Paths) (config.RulesSyncResult, error) {
		called = true
		if cfg.RulesCenter.Release != "latest" {
			t.Fatalf("release should be forced to latest, got %q", cfg.RulesCenter.Release)
		}
		if !cfg.RulesCenter.Strict {
			t.Fatalf("strict should be forced true")
		}
		if _, err := os.Stat(paths.ResolvedRulesDir); !os.IsNotExist(err) {
			t.Fatalf("rules dir should be removed before sync, err=%v", err)
		}
		if _, err := os.Stat(paths.RulesLockPath); !os.IsNotExist(err) {
			t.Fatalf("rules lock should be removed before sync, err=%v", err)
		}
		return config.RulesSyncResult{Updated: true, Message: "规则中心更新成功（latest）"}, nil
	}

	root := NewRootCmd(out, errf)
	root.SetArgs([]string{"update", "rules"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update rules failed: %v", err)
	}
	if !called {
		t.Fatalf("expected syncRulesForUpdate to be called")
	}

	if _, err := out.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "latest" {
		t.Fatalf("unexpected stdout: %q", string(raw))
	}
}

func TestUpdateRulesCommandSyncError(t *testing.T) {
	out, err := os.Create(filepath.Join(t.TempDir(), "stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errf, err := os.Create(filepath.Join(t.TempDir(), "stderr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer errf.Close()

	oldLoad := loadConfigForUpdate
	oldSync := syncRulesForUpdate
	defer func() {
		loadConfigForUpdate = oldLoad
		syncRulesForUpdate = oldSync
	}()
	loadConfigForUpdate = func(pathArg, cwd string) (*config.Config, *config.Paths, error) {
		return &config.Config{}, &config.Paths{
			ResolvedRulesDir: filepath.Join(t.TempDir(), "rules"),
			RulesLockPath:    filepath.Join(t.TempDir(), "rules.lock"),
		}, nil
	}
	syncRulesForUpdate = func(cfg *config.Config, paths *config.Paths) (config.RulesSyncResult, error) {
		return config.RulesSyncResult{}, io.EOF
	}

	root := NewRootCmd(out, errf)
	root.SetArgs([]string{"update", "rules"})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("expected sync error, got %v", err)
	}
}
