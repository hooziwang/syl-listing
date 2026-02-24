package logging

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestGenerateAndTranslateLabelsCoverage(t *testing.T) {
	if label, key, ok := humanGenerateLabel("bullets_item_1"); !ok || label == "" || key != "gen_bullets" {
		t.Fatalf("unexpected bullets_item_1 generate label: %q %q %v", label, key, ok)
	}
	if _, _, ok := humanGenerateLabel("bullets_item_2"); ok {
		t.Fatalf("bullets_item_2 should be hidden in human log")
	}
	if label, _, ok := humanGenerateLabel("search_terms"); !ok || label == "" {
		t.Fatalf("search_terms label missing")
	}

	if label, key, ok := humanTranslateLabel("keywords_batch"); !ok || label == "" || key != "tr_fixed" {
		t.Fatalf("unexpected keywords_batch translate label: %q %q %v", label, key, ok)
	}
	if label, key, ok := humanTranslateLabel("description_batch"); !ok || label == "" || key != "tr_description" {
		t.Fatalf("unexpected description_batch translate label: %q %q %v", label, key, ok)
	}
	if _, _, ok := humanTranslateLabel("bullet_2"); ok {
		t.Fatalf("bullet_2 should be hidden in human log")
	}
}

func TestRequestResponseLabelCoverage(t *testing.T) {
	if label, key, ok := humanRequestLabel("translate_title"); !ok || label == "" || key != "tr_title" {
		t.Fatalf("unexpected translate request label: %q %q %v", label, key, ok)
	}
	if label, key, ok := humanRequestLabel("bullets_item_2"); ok || label != "" || key != "" {
		t.Fatalf("bullets_item_2 request should be hidden")
	}
	if label, key, ok := humanRequestLabel("unknown"); ok || label != "" || key != "" {
		t.Fatalf("unknown request should be hidden")
	}

	if label, key, ok := humanResponseLabel("translate_search_terms"); !ok || label == "" || key != "tr_search_terms" {
		t.Fatalf("unexpected translate response label: %q %q %v", label, key, ok)
	}
	if label, key, ok := humanResponseLabel("unknown"); ok || label != "" || key != "" {
		t.Fatalf("unknown response should be hidden")
	}
}

func TestOnceLineAndIndexedStepIsOneFalse(t *testing.T) {
	l := &Logger{onceKeys: map[string]struct{}{}}
	if got := l.onceLine("k", "line"); got == "" {
		t.Fatalf("first onceLine should emit")
	}
	if got := l.onceLine("k", "line"); got != "" {
		t.Fatalf("second onceLine should be empty, got %q", got)
	}
	if indexedStepIsOne("keyword_2", "keyword_") {
		t.Fatalf("keyword_2 should not be treated as first item")
	}
	if indexedStepIsOne("bad", "keyword_") {
		t.Fatalf("prefix mismatch should be false")
	}
}

func TestLoggerConcurrentOnceBehavior(t *testing.T) {
	var out bytes.Buffer
	l, _, err := New(&out, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Emit(Event{Event: "api_request_title", Input: "/tmp/a.md"})
		}()
	}
	wg.Wait()
	text := out.String()
	if cnt := strings.Count(text, "开始英文标题生成"); cnt != 1 {
		t.Fatalf("expected only one once-line, got %d lines: %s", cnt, text)
	}
}
