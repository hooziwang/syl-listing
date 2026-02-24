package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatHumanDurationMS(t *testing.T) {
	if got := formatHumanDurationMS(-1); got != "0ms" {
		t.Fatalf("unexpected: %s", got)
	}
	if got := formatHumanDurationMS(1500); got != "1.50s" {
		t.Fatalf("unexpected: %s", got)
	}
	if got := formatHumanDurationMS(60_000); got != "1m" {
		t.Fatalf("unexpected: %s", got)
	}
}

func TestLoggerEmitHumanAndNDJSON(t *testing.T) {
	var out bytes.Buffer
	l, _, err := New(&out, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	l.Emit(Event{Event: "api_request_title", Input: "/tmp/a.md"})
	l.Emit(Event{Event: "api_response_title", Input: "/tmp/a.md", LatencyMS: 1200})
	l.Emit(Event{Event: "validation_warning", Input: "/tmp/a.md", Error: "w"})
	text := out.String()
	if !strings.Contains(text, "开始英文标题生成") || !strings.Contains(text, "英文标题生成完成") || !strings.Contains(text, "校验提示") {
		t.Fatalf("unexpected human output: %s", text)
	}

	out.Reset()
	l2, _, err := New(&out, "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	l2.Emit(Event{Event: "startup", Provider: "deepseek"})
	if !strings.Contains(out.String(), "\"event\":\"startup\"") {
		t.Fatalf("unexpected ndjson output: %s", out.String())
	}
}

func TestLabelHelpers(t *testing.T) {
	if lbl, _, ok := humanGenerateLabel("title"); !ok || lbl == "" {
		t.Fatalf("humanGenerateLabel title failed")
	}
	if lbl, _, ok := humanTranslateLabel("keyword_1"); !ok || lbl == "" {
		t.Fatalf("humanTranslateLabel keyword_1 failed")
	}
	if !indexedStepIsOne("keyword_1", "keyword_") {
		t.Fatalf("indexedStepIsOne failed")
	}
	if fallback("", "x") != "x" {
		t.Fatalf("fallback failed")
	}
}
