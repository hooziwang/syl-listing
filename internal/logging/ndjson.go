package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	mu           sync.Mutex
	verbose      bool
	ndjsonWriter io.Writer
	humanWriter  io.Writer
	onceKeys     map[string]struct{}
}

type Event struct {
	TS            string   `json:"ts"`
	Level         string   `json:"level"`
	Event         string   `json:"event"`
	Input         string   `json:"input,omitempty"`
	Candidate     int      `json:"candidate,omitempty"`
	Lang          string   `json:"lang,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Model         string   `json:"model,omitempty"`
	APIMode       string   `json:"api_mode,omitempty"`
	BaseURL       string   `json:"base_url,omitempty"`
	Attempt       int      `json:"attempt,omitempty"`
	WaitMS        int64    `json:"wait_ms,omitempty"`
	LatencyMS     int64    `json:"latency_ms,omitempty"`
	OutputFile    string   `json:"output_file,omitempty"`
	Error         string   `json:"error,omitempty"`
	SystemPrompt  string   `json:"system_prompt,omitempty"`
	UserPrompt    string   `json:"user_prompt,omitempty"`
	SourceText    string   `json:"source_text,omitempty"`
	SourceTexts   []string `json:"source_texts,omitempty"`
	ResponseText  string   `json:"response_text,omitempty"`
	ResponseTexts []string `json:"response_texts,omitempty"`
}

func New(stdout io.Writer, logFile string, verbose bool) (*Logger, io.Closer, error) {
	logger := &Logger{
		verbose:  verbose,
		onceKeys: make(map[string]struct{}),
	}

	if verbose {
		if logFile == "" {
			logger.ndjsonWriter = stdout
			return logger, nil, nil
		}
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, err
		}
		logger.ndjsonWriter = io.MultiWriter(stdout, f)
		return logger, f, nil
	}

	logger.humanWriter = stdout
	if logFile == "" {
		return logger, nil, nil
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	logger.ndjsonWriter = f
	return logger, f, nil
}

func (l *Logger) Verbose() bool {
	return l != nil && l.verbose
}

func (l *Logger) Emit(ev Event) {
	if l == nil {
		return
	}
	if ev.TS == "" {
		ev.TS = time.Now().Format(time.RFC3339Nano)
	}
	if ev.Level == "" {
		ev.Level = "info"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.ndjsonWriter != nil {
		b, err := json.Marshal(ev)
		if err == nil {
			_, _ = l.ndjsonWriter.Write(append(b, '\n'))
		}
	}
	if l.humanWriter != nil {
		line := l.formatHuman(ev)
		if strings.TrimSpace(line) != "" {
			_, _ = io.WriteString(l.humanWriter, line+"\n")
		}
	}
}

func (l *Logger) formatHuman(ev Event) string {
	switch ev.Event {
	case "startup":
		return ""
	case "config_loaded":
		return ""
	case "scan_warning":
		return fmt.Sprintf("扫描警告：%s", fallback(ev.Error, "-"))
	case "parse_failed":
		return fmt.Sprintf("[%s] 解析失败：%s", l.jobTag(ev), fallback(ev.Error, "-"))
	case "validation_failed":
		return fmt.Sprintf("[%s] 校验失败：%s", l.jobTag(ev), fallback(ev.Error, "-"))
	case "validation_warning":
		return fmt.Sprintf("[%s] 校验提示：%s", l.jobTag(ev), fallback(ev.Error, "-"))
	case "name_failed":
		return fmt.Sprintf("[%s] 输出文件名分配失败：%s", l.jobTag(ev), fallback(ev.Error, "-"))
	case "generate_failed":
		return fmt.Sprintf("[%s] 生成失败：%s", l.jobTag(ev), fallback(ev.Error, "-"))
	case "generate_ok":
		return fmt.Sprintf("[%s] %s生成完成（%dms）", l.jobTag(ev), strings.ToUpper(fallback(ev.Lang, "-")), ev.LatencyMS)
	case "write_failed":
		return fmt.Sprintf("[%s] %s 写入失败：%s", l.jobTag(ev), fallback(ev.OutputFile, "-"), fallback(ev.Error, "-"))
	case "write_ok":
		return fmt.Sprintf("[%s] %s 已写入：%s", l.jobTag(ev), strings.ToUpper(fallback(ev.Lang, "-")), fallback(ev.OutputFile, "-"))
	case "finished":
		return ""
	}

	if strings.HasPrefix(ev.Event, "api_request_") {
		return l.humanAPIRequest(ev)
	}
	if strings.HasPrefix(ev.Event, "api_response_") {
		return l.humanAPIResponse(ev)
	}
	if strings.HasPrefix(ev.Event, "retry_backoff_") {
		step := strings.TrimPrefix(ev.Event, "retry_backoff_")
		return fmt.Sprintf("[%s] %s 重试等待 %dms：%s", l.jobTag(ev), humanStepLabel(step), ev.WaitMS, fallback(ev.Error, "-"))
	}
	if strings.HasPrefix(ev.Event, "api_error_") {
		step := strings.TrimPrefix(ev.Event, "api_error_")
		return fmt.Sprintf("[%s] %s 请求失败：%s", l.jobTag(ev), humanStepLabel(step), fallback(ev.Error, "-"))
	}
	if strings.HasPrefix(ev.Event, "validate_error_") {
		step := strings.TrimPrefix(ev.Event, "validate_error_")
		return fmt.Sprintf("[%s] %s 校验失败：%s", l.jobTag(ev), humanStepLabel(step), fallback(ev.Error, "-"))
	}
	return ""
}

func (l *Logger) humanAPIRequest(ev Event) string {
	step := strings.TrimPrefix(ev.Event, "api_request_")
	label, groupKey, show := humanRequestLabel(step)
	if !show {
		return ""
	}
	if ev.Attempt > 1 {
		return fmt.Sprintf("[%s] %s（第%d次）", l.jobTag(ev), label, ev.Attempt)
	}
	return l.onceLine(l.jobTag(ev)+":req:"+groupKey, fmt.Sprintf("[%s] %s", l.jobTag(ev), label))
}

func (l *Logger) humanAPIResponse(ev Event) string {
	step := strings.TrimPrefix(ev.Event, "api_response_")
	label, groupKey, show := humanResponseLabel(step)
	if !show {
		return ""
	}
	return l.onceLine(
		l.jobTag(ev)+":resp:"+groupKey,
		fmt.Sprintf("[%s] %s（%dms）", l.jobTag(ev), label, ev.LatencyMS),
	)
}

func (l *Logger) onceLine(key, line string) string {
	if _, ok := l.onceKeys[key]; ok {
		return ""
	}
	l.onceKeys[key] = struct{}{}
	return line
}

func (l *Logger) jobTag(ev Event) string {
	base := strings.TrimSpace(ev.Input)
	if base == "" {
		base = "-"
	} else {
		base = filepath.Base(base)
	}
	if ev.Candidate > 0 {
		return fmt.Sprintf("%s#%d", base, ev.Candidate)
	}
	return base
}

func humanRequestLabel(step string) (string, string, bool) {
	if strings.HasPrefix(step, "translate_") {
		label, key, ok := humanTranslateLabel(strings.TrimPrefix(step, "translate_"))
		if !ok {
			return "", "", false
		}
		return "开始" + label, key, true
	}
	label, key, ok := humanGenerateLabel(step)
	if !ok {
		return "", "", false
	}
	return "开始" + label, key, true
}

func humanResponseLabel(step string) (string, string, bool) {
	if strings.HasPrefix(step, "translate_") {
		label, key, ok := humanTranslateLabel(strings.TrimPrefix(step, "translate_"))
		if !ok {
			return "", "", false
		}
		return label + "完成", key, true
	}
	label, key, ok := humanGenerateLabel(step)
	if !ok {
		return "", "", false
	}
	return label + "完成", key, true
}

func humanGenerateLabel(step string) (string, string, bool) {
	switch {
	case step == "title":
		return "英文标题生成", "gen_title", true
	case step == "bullets", strings.HasPrefix(step, "bullets_item_"):
		return "英文五点描述生成", "gen_bullets", step == "bullets" || indexedStepIsOne(step, "bullets_item_")
	case step == "description":
		return "英文产品描述生成", "gen_description", true
	case step == "search_terms":
		return "英文搜索词生成", "gen_search_terms", true
	default:
		return "", "", false
	}
}

func humanTranslateLabel(step string) (string, string, bool) {
	switch {
	case step == "title":
		return "中文标题翻译", "tr_title", true
	case step == "category", step == "keywords_batch", strings.HasPrefix(step, "keyword_"):
		return "中文分类与关键词翻译", "tr_fixed", step == "category" || step == "keywords_batch" || indexedStepIsOne(step, "keyword_")
	case strings.HasPrefix(step, "bullet_"), step == "bullets_batch":
		return "中文五点描述翻译", "tr_bullets", step == "bullets_batch" || indexedStepIsOne(step, "bullet_")
	case strings.HasPrefix(step, "description_"), step == "description_batch":
		return "中文产品描述翻译", "tr_description", step == "description_batch" || indexedStepIsOne(step, "description_")
	case step == "search_terms":
		return "中文搜索词翻译", "tr_search_terms", true
	default:
		return "", "", false
	}
}

func humanStepLabel(step string) string {
	if strings.HasPrefix(step, "translate_") {
		label, _, ok := humanTranslateLabel(strings.TrimPrefix(step, "translate_"))
		if ok {
			return label
		}
	}
	label, _, ok := humanGenerateLabel(step)
	if ok {
		return label
	}
	return step
}

func fallback(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func indexedStepIsOne(step, prefix string) bool {
	if !strings.HasPrefix(step, prefix) {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(step, prefix)) == "1"
}
