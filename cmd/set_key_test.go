package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetKeyCommandCreateAndUpdateEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

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

	root := NewRootCmd(out, errf)
	root.SetArgs([]string{"set", "key", "first-key"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set key create failed: %v", err)
	}
	envPath := filepath.Join(home, ".syl-listing", ".env")
	raw, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "DEEPSEEK_API_KEY=first-key") {
		t.Fatalf("missing key after create: %s", text)
	}

	root = NewRootCmd(out, errf)
	root.SetArgs([]string{"set", "key", "second-key"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set key update failed: %v", err)
	}
	raw, err = os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	text = string(raw)
	if !strings.Contains(text, "DEEPSEEK_API_KEY=second-key") || strings.Contains(text, "DEEPSEEK_API_KEY=first-key") {
		t.Fatalf("key not updated correctly: %s", text)
	}

	if _, err := out.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	stdoutRaw, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(stdoutRaw)) != "" {
		t.Fatalf("expected empty stdout, got: %q", string(stdoutRaw))
	}
	if _, err := errf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	stderrRaw, err := io.ReadAll(errf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(stderrRaw)) != "" {
		t.Fatalf("expected empty stderr, got: %q", string(stderrRaw))
	}
}

func TestSetKeyCommandEmptyKey(t *testing.T) {
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

	root := NewRootCmd(out, errf)
	root.SetArgs([]string{"set", "key", "   "})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "API Key 不能为空") {
		t.Fatalf("expected empty key error, got %v", err)
	}
}
