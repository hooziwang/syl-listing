package logging

import "testing"

func TestFormatHumanSwitchCases(t *testing.T) {
	l := &Logger{showCandidate: true, onceKeys: map[string]struct{}{}}
	cases := []string{
		"startup",
		"config_loaded",
		"rules_sync_updated",
		"rules_sync_warning",
		"scan_warning",
		"parse_failed",
		"validation_failed",
		"validation_warning",
		"name_failed",
		"generate_failed",
		"generate_ok",
		"write_failed",
		"write_ok",
		"balance",
		"balance_failed",
		"finished",
	}
	for _, c := range cases {
		_ = l.formatHuman(Event{Event: c, Input: "/tmp/a.md", Candidate: 1, Lang: "en", Error: "err", LatencyMS: 1000, OutputFile: "f"})
	}
}
