package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormatDurationMS(t *testing.T) {
	cases := map[int64]string{
		-1:    "0ms",
		999:   "999ms",
		1000:  "1.00s",
		61000: "1m1.0s",
	}
	for in, want := range cases {
		if got := formatDurationMS(in); got != want {
			t.Fatalf("%d => %s, want %s", in, got, want)
		}
	}
}

func TestFormatSummaryBalance(t *testing.T) {
	if got := formatSummaryBalance(" "); got != "查询失败" {
		t.Fatalf("unexpected: %s", got)
	}
	if got := formatSummaryBalance(" CNY 1.2 "); got != "CNY 1.2" {
		t.Fatalf("unexpected: %s", got)
	}
}

func TestNormalizeArgs(t *testing.T) {
	if got := normalizeArgs([]string{"a.md"}); !reflect.DeepEqual(got, []string{"gen", "a.md"}) {
		t.Fatalf("unexpected: %#v", got)
	}
	if got := normalizeArgs([]string{"gen", "a.md"}); !reflect.DeepEqual(got, []string{"gen", "a.md"}) {
		t.Fatalf("unexpected: %#v", got)
	}
	if got := normalizeArgs([]string{"--config", "x"}); !reflect.DeepEqual(got, []string{"--config", "x"}) {
		t.Fatalf("unexpected: %#v", got)
	}
	if got := normalizeArgs([]string{"-v"}); !reflect.DeepEqual(got, []string{"-v"}) {
		t.Fatalf("unexpected: %#v", got)
	}
	if got := normalizeArgs([]string{"set", "key", "abc"}); !reflect.DeepEqual(got, []string{"set", "key", "abc"}) {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestContainsPositionalSource(t *testing.T) {
	if containsPositionalSource([]string{"--config", "x"}) {
		t.Fatalf("unexpected true")
	}
	if !containsPositionalSource([]string{"--config", "x", "a.md"}) {
		t.Fatalf("expected true")
	}
	if !containsPositionalSource([]string{"--", "a.md"}) {
		t.Fatalf("expected true")
	}
}

func TestVersionText(t *testing.T) {
	oldV, oldC, oldB := Version, Commit, BuildTime
	defer func() {
		Version, Commit, BuildTime = oldV, oldC, oldB
	}()
	Version, Commit, BuildTime = "v1", "abc", "t"
	out := versionText()
	if !strings.Contains(out, "v1") || !strings.Contains(out, "abc") {
		t.Fatalf("unexpected version text: %s", out)
	}
}
