package config

import (
	"os"
	"path/filepath"
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
