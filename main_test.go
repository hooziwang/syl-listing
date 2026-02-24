package main

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestMainSuccessWithVersion(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"syl-listing", "--version"}
	main()
}

func TestMainExitOnExecuteError(t *testing.T) {
	if os.Getenv("TEST_MAIN_EXIT") == "1" {
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()
		os.Args = []string{"syl-listing", "--definitely-bad-flag"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainExitOnExecuteError")
	cmd.Env = append(os.Environ(), "TEST_MAIN_EXIT=1")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected exit code 1")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}
}
