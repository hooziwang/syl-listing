package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, ".env")
	content := "\n# comment\nA=1\nB = ' two ' \nC=\"three\"\nINVALID\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadEnvFile(p)
	if err != nil {
		t.Fatalf("LoadEnvFile error: %v", err)
	}
	if m["A"] != "1" || m["B"] != " two " || m["C"] != "three" {
		t.Fatalf("unexpected map: %#v", m)
	}
}

func TestUpsertEnvVar(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, ".env")
	if err := UpsertEnvVar(p, "DEEPSEEK_API_KEY", "k1"); err != nil {
		t.Fatalf("UpsertEnvVar create failed: %v", err)
	}
	m, err := LoadEnvFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if m["DEEPSEEK_API_KEY"] != "k1" {
		t.Fatalf("unexpected created value: %#v", m)
	}

	raw := "A=1\nDEEPSEEK_API_KEY=old\n#comment\n"
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertEnvVar(p, "DEEPSEEK_API_KEY", "k2"); err != nil {
		t.Fatalf("UpsertEnvVar update failed: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "DEEPSEEK_API_KEY=k2") || strings.Contains(text, "DEEPSEEK_API_KEY=old") {
		t.Fatalf("unexpected updated file: %s", text)
	}
}

func TestUpsertEnvVarErrors(t *testing.T) {
	if err := UpsertEnvVar(filepath.Join(t.TempDir(), ".env"), " ", "x"); err == nil {
		t.Fatalf("expected empty key error")
	}

	d := t.TempDir()
	blocker := filepath.Join(d, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertEnvVar(filepath.Join(blocker, ".env"), "K", "V"); err == nil {
		t.Fatalf("expected mkdir failure")
	}
}
