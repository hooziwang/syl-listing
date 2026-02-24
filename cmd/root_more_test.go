package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintVersionAndLoveBanner(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	printVersion(f)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "syl-listing 版本") {
		t.Fatalf("unexpected version output: %s", text)
	}
}

func TestRootCmdVersionFlagAndNoArgsHelp(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.txt")
	errPath := filepath.Join(t.TempDir(), "err.txt")
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errf, err := os.Create(errPath)
	if err != nil {
		t.Fatal(err)
	}
	defer errf.Close()

	root := NewRootCmd(out, errf)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute --version failed: %v", err)
	}
	root = NewRootCmd(out, errf)
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute with no args failed: %v", err)
	}
}

func TestExecuteWithVersionArg(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"syl-listing", "--version"}
	if err := Execute(); err != nil {
		t.Fatalf("Execute --version failed: %v", err)
	}
}
