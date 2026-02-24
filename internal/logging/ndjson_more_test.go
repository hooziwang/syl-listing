package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerNewWithLogFileAndVerboseMethod(t *testing.T) {
	d := t.TempDir()
	logPath := filepath.Join(d, "run.log")
	var out bytes.Buffer
	l, closer, err := New(&out, logPath, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if l.Verbose() {
		t.Fatalf("verbose should be false")
	}
	l.Emit(Event{Event: "write_ok", Input: "/tmp/a.md", Candidate: 2, Lang: "en", OutputFile: "x"})
	if closer != nil {
		_ = closer.Close()
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\"event\":\"write_ok\"") {
		t.Fatalf("expected ndjson file output, got %s", string(raw))
	}
}

func TestHumanLabelsAndStepLabel(t *testing.T) {
	cases := []string{
		"api_request_title", "api_response_title", "retry_backoff_title", "api_error_title", "validate_error_title",
		"thinking_fallback_title", "rules_sync_updated", "rules_sync_warning", "generate_ok", "write_ok",
	}
	l := &Logger{showCandidate: true, onceKeys: map[string]struct{}{}}
	for _, ev := range cases {
		line := l.formatHuman(Event{Event: ev, Input: "/tmp/a.md", Candidate: 2, Lang: "en", Error: "e", LatencyMS: 1200})
		if ev == "generate_ok" && !strings.Contains(line, "生成完成") {
			t.Fatalf("unexpected generate_ok line: %s", line)
		}
	}
	if humanStepLabel("bullets_item_2") != "英文五点描述生成" {
		t.Fatalf("humanStepLabel bullets mismatch")
	}
	if humanStepLabel("translate_keyword_2") != "中文分类与关键词翻译" {
		t.Fatalf("humanStepLabel keyword mismatch")
	}
	if humanStepLabel("translate_bullet_2") != "中文五点描述翻译" {
		t.Fatalf("humanStepLabel bullet mismatch")
	}
	if humanStepLabel("translate_description_1") != "中文产品描述翻译" {
		t.Fatalf("humanStepLabel description mismatch")
	}
	if humanStepLabel("translate_title") != "中文标题翻译" {
		t.Fatalf("humanStepLabel title mismatch")
	}
}

func TestRequestResponseLabelHelpers(t *testing.T) {
	if label, _, ok := humanRequestLabel("title"); !ok || !strings.Contains(label, "开始") {
		t.Fatalf("humanRequestLabel title failed")
	}
	if label, _, ok := humanResponseLabel("translate_title"); !ok || !strings.Contains(label, "完成") {
		t.Fatalf("humanResponseLabel translate failed")
	}
	if _, _, ok := humanGenerateLabel("unknown"); ok {
		t.Fatalf("expected unknown generate label")
	}
	if _, _, ok := humanTranslateLabel("unknown"); ok {
		t.Fatalf("expected unknown translate label")
	}
	if line := (&Logger{onceKeys: map[string]struct{}{}}).formatHuman(Event{Event: "unknown"}); line != "" {
		t.Fatalf("unknown event should return empty line")
	}
}
