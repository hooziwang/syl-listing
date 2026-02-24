package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/logging"
)

type sectionGenerateOptions struct {
	Req           listing.Requirement
	Lang          string
	CharTolerance int
	Provider      string
	ProviderCfg   config.ProviderConfig
	APIKey        string
	Rules         config.SectionRules
	MaxRetries    int
	Client        *llm.Client
	Logger        *logging.Logger
	Candidate     int
}

func generateDocumentBySections(opts sectionGenerateOptions) (ListingDocument, int64, error) {
	doc := ListingDocument{
		Keywords: append([]string{}, opts.Req.Keywords...),
		Category: strings.TrimSpace(opts.Req.Category),
	}
	var total int64

	title, latency, err := generateSectionWithRetry(opts, "title", doc)
	total += latency
	if err != nil {
		return ListingDocument{}, total, err
	}
	doc.Title = cleanTitleLine(title)

	bulletRule, err := opts.Rules.Get("bullets")
	if err != nil {
		return ListingDocument{}, total, err
	}
	var bullets []string
	if strings.EqualFold(opts.Provider, "deepseek") {
		items, itemLatency, itemErr := generateBulletsWithJSONAndRepair(opts, doc, bulletRule)
		total += itemLatency
		if itemErr != nil {
			return ListingDocument{}, total, itemErr
		}
		bullets = items
	} else {
		bulletsText, sectionLatency, sectionErr := generateSectionWithRetry(opts, "bullets", doc)
		total += sectionLatency
		if sectionErr != nil {
			return ListingDocument{}, total, sectionErr
		}
		items, parseErr := parseBullets(bulletsText, bulletRule.Parsed.Output.Lines)
		if parseErr != nil {
			return ListingDocument{}, total, parseErr
		}
		bullets = items
	}
	doc.BulletPoints = bullets

	descText, latency, err := generateSectionWithRetry(opts, "description", doc)
	total += latency
	if err != nil {
		return ListingDocument{}, total, err
	}
	descRule, err := opts.Rules.Get("description")
	if err != nil {
		return ListingDocument{}, total, err
	}
	desc, err := parseParagraphs(descText, descRule.Parsed.Output.Paragraphs)
	if err != nil {
		return ListingDocument{}, total, err
	}
	doc.DescriptionParagraphs = desc

	search, latency, err := generateSectionWithRetry(opts, "search_terms", doc)
	total += latency
	if err != nil {
		return ListingDocument{}, total, err
	}
	doc.SearchTerms = cleanSearchTermsLine(search)

	return doc, total, validateDocumentBySectionRules(opts.Lang, opts.Req, doc, opts.Rules)
}

func generateSectionWithRetry(opts sectionGenerateOptions, step string, doc ListingDocument) (string, int64, error) {
	var (
		outText    string
		outLatency int64
	)
	lastIssues := ""
	history := make([]llm.Message, 0, 6)
	err := withExponentialBackoff(retryOptions{
		MaxRetries: opts.MaxRetries,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   8 * time.Second,
		Jitter:     0.25,
		OnRetry: func(attempt int, wait time.Duration, err error) {
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "retry_backoff_" + step,
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				WaitMS:    wait.Milliseconds(),
				Error:     err.Error(),
			})
		},
	}, func(attempt int) error {
		sectionRule, err := opts.Rules.Get(step)
		if err != nil {
			lastIssues = "- 读取分段规则失败: " + err.Error()
			return errors.New(lastIssues)
		}
		systemPrompt := buildSectionSystemPrompt(sectionRule)
		baseUserPrompt := buildSectionUserPrompt(step, opts.Req, doc)
		messages := make([]llm.Message, 0, 2+len(history))
		messages = append(messages,
			llm.Message{Role: "system", Content: systemPrompt},
			llm.Message{Role: "user", Content: baseUserPrompt},
		)
		messages = append(messages, history...)

		reqEvent := logging.Event{
			Event:     "api_request_" + step,
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			APIMode:   opts.ProviderCfg.APIMode,
			BaseURL:   opts.ProviderCfg.BaseURL,
			Attempt:   attempt,
		}
		reqModel, thinkingFallback := resolveModelForAttempt(opts, attempt)
		reqEvent.Model = reqModel
		if thinkingFallback {
			opts.Logger.Emit(logging.Event{
				Event:     "thinking_fallback_" + step,
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Provider:  opts.Provider,
				Model:     reqModel,
				Attempt:   attempt,
			})
		}
		if opts.Logger.Verbose() {
			reqEvent.SystemPrompt = systemPrompt
			reqEvent.UserPrompt = baseUserPrompt
		}
		opts.Logger.Emit(reqEvent)
		resp, err := opts.Client.Generate(context.Background(), llm.Request{
			Provider:        opts.Provider,
			BaseURL:         opts.ProviderCfg.BaseURL,
			Model:           reqModel,
			APIMode:         opts.ProviderCfg.APIMode,
			APIKey:          opts.APIKey,
			ReasoningEffort: opts.ProviderCfg.ModelReasoningEffort,
			SystemPrompt:    systemPrompt,
			UserPrompt:      baseUserPrompt,
			Messages:        messages,
		})
		if err != nil {
			lastIssues = "- API 调用失败: " + err.Error()
			opts.Logger.Emit(logging.Event{Level: "warn", Event: "api_error_" + step, Input: opts.Req.SourcePath, Candidate: opts.Candidate, Lang: opts.Lang, Attempt: attempt, Error: err.Error()})
			return errors.New(lastIssues)
		}

		text := normalizeModelText(resp.Text)
		respEvent := logging.Event{
			Event:     "api_response_" + step,
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			Model:     reqModel,
			LatencyMS: resp.LatencyMS,
			Attempt:   attempt,
		}
		if opts.Logger.Verbose() {
			respEvent.ResponseText = text
		}
		opts.Logger.Emit(respEvent)
		if step == "search_terms" {
			text = cleanSearchTermsLine(text)
		}
		issues, warnings := validateSectionText(step, opts.Lang, opts.Req, text, sectionRule, opts.CharTolerance)
		for _, w := range warnings {
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "validation_warning",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     w,
			})
		}
		if len(issues) > 0 {
			lastIssues = "- " + strings.Join(issues, "\n- ")
			history = append(history,
				llm.Message{Role: "assistant", Content: text},
				llm.Message{Role: "user", Content: buildSectionRepairPrompt(step, issues)},
			)
			opts.Logger.Emit(logging.Event{Level: "warn", Event: "validate_error_" + step, Input: opts.Req.SourcePath, Candidate: opts.Candidate, Lang: opts.Lang, Attempt: attempt, Error: strings.Join(issues, "; ")})
			return errors.New(strings.Join(issues, "; "))
		}
		outText = text
		outLatency = resp.LatencyMS
		return nil
	})
	if err != nil {
		if strings.TrimSpace(lastIssues) == "" {
			lastIssues = err.Error()
		}
		return "", 0, fmt.Errorf("%s 重试后仍失败: %s", step, lastIssues)
	}
	return outText, outLatency, nil
}

func generateBulletsWithJSONAndRepair(opts sectionGenerateOptions, doc ListingDocument, rule config.SectionRuleFile) ([]string, int64, error) {
	expected := rule.Parsed.Output.Lines
	if expected <= 0 {
		return nil, 0, fmt.Errorf("bullets 规则 output.lines 无效：%d", expected)
	}
	bounds := resolveCharBounds(
		rule.Parsed.Constraints.MinCharsPerLine.Value,
		rule.Parsed.Constraints.MaxCharsPerLine.Value,
		opts.CharTolerance,
	)
	var total int64

	out, latencyMS, err := generateBulletsBatchJSONWithRetry(opts, doc, rule, bounds)
	total += latencyMS
	if err != nil {
		return nil, total, err
	}

	invalidIndexes, issues, warnings := validateBulletSet(out, bounds)
	for _, w := range warnings {
		opts.Logger.Emit(logging.Event{
			Level:     "warn",
			Event:     "validation_warning",
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Error:     w,
		})
	}
	if len(issues) > 0 && len(invalidIndexes) == 0 {
		return nil, total, fmt.Errorf("bullets 校验失败：%s", strings.Join(issues, "; "))
	}
	if len(invalidIndexes) == 0 {
		return out, total, nil
	}

	snapshot := append([]string{}, out...)
	repaired := append([]string{}, out...)
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		firstErr  error
		repairSum int64
	)
	for _, idx := range invalidIndexes {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			line, latency, itemErr := regenerateBulletItemJSONWithRetry(opts, doc, rule, idx, snapshot, bounds)
			mu.Lock()
			defer mu.Unlock()
			repairSum += latency
			if itemErr != nil {
				if firstErr == nil {
					firstErr = itemErr
				}
				return
			}
			repaired[idx-1] = line
		}()
	}
	wg.Wait()
	total += repairSum
	if firstErr != nil {
		return nil, total, firstErr
	}

	_, finalIssues, _ := validateBulletSet(repaired, bounds)
	if len(finalIssues) > 0 {
		return nil, total, fmt.Errorf("bullets 修复后仍不满足规则：%s", strings.Join(finalIssues, "; "))
	}
	return repaired, total, nil
}

type bulletsBatchJSON struct {
	Bullets []string `json:"bullets"`
	Items   []string `json:"items"`
}

type bulletItemJSON struct {
	Bullet string `json:"bullet"`
	Text   string `json:"text"`
	Item   string `json:"item"`
}

func generateBulletsBatchJSONWithRetry(opts sectionGenerateOptions, doc ListingDocument, rule config.SectionRuleFile, bounds charBounds) ([]string, int64, error) {
	expected := rule.Parsed.Output.Lines
	var (
		outItems   []string
		outLatency int64
	)
	lastIssues := ""
	systemPrompt := buildSectionSystemPrompt(rule) + `

【JSON协议】
Return valid json only.
必须只返回一个 json object，禁止 markdown 代码块、禁止解释。
对象结构固定为：{"bullets":["line1","line2","line3","line4","line5"]}`
	baseUserPrompt := buildSectionUserPrompt("bullets", opts.Req, doc) +
		fmt.Sprintf("\n【输出要求】必须返回 json object，键名是 bullets，且 bullets 必须恰好 %d 条。", expected)
	history := make([]llm.Message, 0, 8)
	err := withExponentialBackoff(retryOptions{
		MaxRetries: opts.MaxRetries,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   8 * time.Second,
		Jitter:     0.25,
		OnRetry: func(attempt int, wait time.Duration, err error) {
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "retry_backoff_bullets",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				WaitMS:    wait.Milliseconds(),
				Error:     err.Error(),
			})
		},
	}, func(attempt int) error {
		messages := make([]llm.Message, 0, 2+len(history))
		messages = append(messages,
			llm.Message{Role: "system", Content: systemPrompt},
			llm.Message{Role: "user", Content: baseUserPrompt},
		)
		messages = append(messages, history...)

		reqEvent := logging.Event{
			Event:     "api_request_bullets",
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			APIMode:   opts.ProviderCfg.APIMode,
			BaseURL:   opts.ProviderCfg.BaseURL,
			Attempt:   attempt,
		}
		reqModel, thinkingFallback := resolveModelForAttempt(opts, attempt)
		reqEvent.Model = reqModel
		if thinkingFallback {
			opts.Logger.Emit(logging.Event{
				Event:     "thinking_fallback_bullets",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Provider:  opts.Provider,
				Model:     reqModel,
				Attempt:   attempt,
			})
		}
		if opts.Logger.Verbose() {
			reqEvent.SystemPrompt = systemPrompt
			reqEvent.UserPrompt = baseUserPrompt
		}
		opts.Logger.Emit(reqEvent)
		resp, err := opts.Client.Generate(context.Background(), llm.Request{
			Provider:        opts.Provider,
			BaseURL:         opts.ProviderCfg.BaseURL,
			Model:           reqModel,
			APIMode:         opts.ProviderCfg.APIMode,
			APIKey:          opts.APIKey,
			ReasoningEffort: opts.ProviderCfg.ModelReasoningEffort,
			SystemPrompt:    systemPrompt,
			UserPrompt:      baseUserPrompt,
			Messages:        messages,
			JSONMode:        true,
		})
		if err != nil {
			lastIssues = "- API 调用失败: " + err.Error()
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "api_error_bullets",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     err.Error(),
			})
			return errors.New(lastIssues)
		}
		text := normalizeModelText(resp.Text)
		respEvent := logging.Event{
			Event:     "api_response_bullets",
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			Model:     reqModel,
			LatencyMS: resp.LatencyMS,
			Attempt:   attempt,
		}
		if opts.Logger.Verbose() {
			respEvent.ResponseText = text
		}
		opts.Logger.Emit(respEvent)

		items, parseErr := parseBulletsFromJSON(text, expected)
		if parseErr != nil {
			lastIssues = "- JSON 解析失败: " + parseErr.Error()
			history = append(history,
				llm.Message{Role: "assistant", Content: text},
				llm.Message{Role: "user", Content: buildJSONRepairPrompt(parseErr.Error(), `{"bullets":["..."]}`)},
			)
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "validate_error_bullets",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     parseErr.Error(),
			})
			return errors.New(lastIssues)
		}
		outItems = items
		outLatency = resp.LatencyMS
		return nil
	})
	if err != nil {
		if strings.TrimSpace(lastIssues) == "" {
			lastIssues = err.Error()
		}
		return nil, 0, fmt.Errorf("bullets 批量生成重试后仍失败: %s", lastIssues)
	}
	return outItems, outLatency, nil
}

func regenerateBulletItemJSONWithRetry(opts sectionGenerateOptions, doc ListingDocument, rule config.SectionRuleFile, idx int, current []string, bounds charBounds) (string, int64, error) {
	var (
		outLine    string
		outLatency int64
	)
	lastIssues := ""
	systemPrompt := buildSectionSystemPrompt(rule) + `

【JSON协议】
Return valid json only.
必须只返回一个 json object：{"bullet":"..."}`
	tmpDoc := doc
	tmpDoc.BulletPoints = append([]string{}, current...)
	baseUserPrompt := buildSectionUserPrompt("bullets", opts.Req, tmpDoc) +
		fmt.Sprintf("\n【子任务】只修复第%d条，返回 json object：{\"bullet\":\"...\"}。", idx)
	history := make([]llm.Message, 0, 8)
	err := withExponentialBackoff(retryOptions{
		MaxRetries: opts.MaxRetries,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   8 * time.Second,
		Jitter:     0.25,
		OnRetry: func(attempt int, wait time.Duration, err error) {
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     fmt.Sprintf("retry_backoff_bullets_item_%d", idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				WaitMS:    wait.Milliseconds(),
				Error:     err.Error(),
			})
		},
	}, func(attempt int) error {
		messages := make([]llm.Message, 0, 2+len(history))
		messages = append(messages,
			llm.Message{Role: "system", Content: systemPrompt},
			llm.Message{Role: "user", Content: baseUserPrompt},
		)
		messages = append(messages, history...)

		reqEvent := logging.Event{
			Event:     fmt.Sprintf("api_request_bullets_item_%d", idx),
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			APIMode:   opts.ProviderCfg.APIMode,
			BaseURL:   opts.ProviderCfg.BaseURL,
			Attempt:   attempt,
		}
		reqModel, thinkingFallback := resolveModelForAttempt(opts, attempt)
		reqEvent.Model = reqModel
		if thinkingFallback {
			opts.Logger.Emit(logging.Event{
				Event:     fmt.Sprintf("thinking_fallback_bullets_item_%d", idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Provider:  opts.Provider,
				Model:     reqModel,
				Attempt:   attempt,
			})
		}
		if opts.Logger.Verbose() {
			reqEvent.SystemPrompt = systemPrompt
			reqEvent.UserPrompt = baseUserPrompt
		}
		opts.Logger.Emit(reqEvent)
		resp, err := opts.Client.Generate(context.Background(), llm.Request{
			Provider:        opts.Provider,
			BaseURL:         opts.ProviderCfg.BaseURL,
			Model:           reqModel,
			APIMode:         opts.ProviderCfg.APIMode,
			APIKey:          opts.APIKey,
			ReasoningEffort: opts.ProviderCfg.ModelReasoningEffort,
			SystemPrompt:    systemPrompt,
			UserPrompt:      baseUserPrompt,
			Messages:        messages,
			JSONMode:        true,
		})
		if err != nil {
			lastIssues = "- API 调用失败: " + err.Error()
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     fmt.Sprintf("api_error_bullets_item_%d", idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     err.Error(),
			})
			return errors.New(lastIssues)
		}

		text := normalizeModelText(resp.Text)
		respEvent := logging.Event{
			Event:     fmt.Sprintf("api_response_bullets_item_%d", idx),
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			Model:     reqModel,
			LatencyMS: resp.LatencyMS,
			Attempt:   attempt,
		}
		if opts.Logger.Verbose() {
			respEvent.ResponseText = text
		}
		opts.Logger.Emit(respEvent)

		line, parseErr := parseBulletItemFromJSON(text)
		if parseErr != nil {
			lastIssues = "- JSON 解析失败: " + parseErr.Error()
			history = append(history,
				llm.Message{Role: "assistant", Content: text},
				llm.Message{Role: "user", Content: buildJSONRepairPrompt(parseErr.Error(), `{"bullet":"..."}`)},
			)
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     fmt.Sprintf("validate_error_bullets_item_%d", idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     parseErr.Error(),
			})
			return errors.New(lastIssues)
		}

		issues, warnings := validateBulletLine(idx, line, bounds)
		for _, w := range warnings {
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "validation_warning",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     w,
			})
		}
		if len(issues) > 0 {
			lastIssues = "- " + strings.Join(issues, "\n- ")
			history = append(history,
				llm.Message{Role: "assistant", Content: text},
				llm.Message{Role: "user", Content: buildJSONRepairPrompt(strings.Join(issues, "; "), `{"bullet":"..."}`)},
			)
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     fmt.Sprintf("validate_error_bullets_item_%d", idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     strings.Join(issues, "; "),
			})
			return errors.New(strings.Join(issues, "; "))
		}
		outLine = cleanBulletLine(line)
		outLatency = resp.LatencyMS
		return nil
	})
	if err != nil {
		if strings.TrimSpace(lastIssues) == "" {
			lastIssues = err.Error()
		}
		return "", 0, fmt.Errorf("bullets 第%d条修复重试后仍失败: %s", idx, lastIssues)
	}
	return outLine, outLatency, nil
}

func validateBulletSet(items []string, bounds charBounds) ([]int, []string, []string) {
	invalid := make([]int, 0, len(items))
	issues := make([]string, 0, len(items))
	warnings := make([]string, 0, len(items))
	for i, it := range items {
		lineIssues, lineWarnings := validateBulletLine(i+1, it, bounds)
		if len(lineIssues) > 0 {
			invalid = append(invalid, i+1)
			issues = append(issues, lineIssues...)
		}
		if len(lineWarnings) > 0 {
			warnings = append(warnings, lineWarnings...)
		}
	}
	return invalid, dedupeIssues(issues), dedupeIssues(warnings)
}

func validateBulletLine(idx int, raw string, bounds charBounds) ([]string, []string) {
	issues := make([]string, 0, 3)
	warnings := make([]string, 0, 1)
	if countNonEmptyLines(raw) != 1 {
		issues = append(issues, fmt.Sprintf("第%d条应为单行输出", idx))
	}
	line := cleanBulletLine(raw)
	if strings.TrimSpace(line) == "" {
		issues = append(issues, fmt.Sprintf("第%d条为空", idx))
		return issues, warnings
	}
	n := runeLen(line)
	if bounds.hasRule() && !bounds.inRule(n) {
		if bounds.inTolerance(n) {
			warnings = append(warnings, fmt.Sprintf("第%d条长度 %d 未落入规则区间 %s，但落入容差区间 %s，已放行", idx, n, bounds.ruleText(), bounds.toleranceText()))
		} else {
			issues = append(issues, fmt.Sprintf("第%d条长度超出容差区间：%d 不在 %s（规则区间 %s）", idx, n, bounds.toleranceText(), bounds.ruleText()))
		}
	}
	return issues, warnings
}

func parseBulletsFromJSON(text string, expected int) ([]string, error) {
	var payload bulletsBatchJSON
	if err := decodeJSONObject(text, &payload); err != nil {
		return nil, err
	}
	items := payload.Bullets
	if len(items) == 0 {
		items = payload.Items
	}
	if len(items) != expected {
		return nil, fmt.Errorf("五点数量错误：%d != %d", len(items), expected)
	}
	out := make([]string, 0, expected)
	for i, it := range items {
		clean := cleanBulletLine(it)
		if strings.TrimSpace(clean) == "" {
			return nil, fmt.Errorf("第%d条为空", i+1)
		}
		out = append(out, clean)
	}
	return out, nil
}

func parseBulletItemFromJSON(text string) (string, error) {
	var payload bulletItemJSON
	if err := decodeJSONObject(text, &payload); err != nil {
		return "", err
	}
	line := strings.TrimSpace(payload.Bullet)
	if line == "" {
		line = strings.TrimSpace(payload.Text)
	}
	if line == "" {
		line = strings.TrimSpace(payload.Item)
	}
	if line == "" {
		return "", fmt.Errorf("缺少 bullet 字段")
	}
	return line, nil
}

func decodeJSONObject(raw string, out any) error {
	text := strings.TrimSpace(raw)
	if text == "" {
		return fmt.Errorf("JSON 为空")
	}
	if err := json.Unmarshal([]byte(text), out); err == nil {
		return nil
	}
	obj := extractJSONObject(text)
	if strings.TrimSpace(obj) == "" {
		return fmt.Errorf("未找到有效 JSON 对象")
	}
	if err := json.Unmarshal([]byte(obj), out); err != nil {
		return fmt.Errorf("JSON 解析失败：%w", err)
	}
	return nil
}

func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func buildSectionSystemPrompt(rule config.SectionRuleFile) string {
	return strings.TrimSpace(rule.Raw)
}

func buildSectionUserPrompt(step string, req listing.Requirement, doc ListingDocument) string {
	var b strings.Builder
	b.WriteString("【需求原文】\n")
	b.WriteString(req.BodyAfterMarker)
	b.WriteString("\n\n【固定字段（不得改写）】\n")
	b.WriteString("category: ")
	b.WriteString(strings.TrimSpace(req.Category))
	b.WriteString("\nkeywords:\n")
	for _, kw := range req.Keywords {
		b.WriteString("- ")
		b.WriteString(kw)
		b.WriteString("\n")
	}
	if doc.Title != "" {
		b.WriteString("\n【已生成标题】\n")
		b.WriteString(doc.Title)
		b.WriteString("\n")
	}
	if len(doc.BulletPoints) > 0 {
		b.WriteString("\n【已生成五点】\n")
		for i, bp := range doc.BulletPoints {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, bp))
		}
	}
	if len(doc.DescriptionParagraphs) > 0 {
		b.WriteString("\n【已生成描述】\n")
		for _, p := range doc.DescriptionParagraphs {
			b.WriteString(p)
			b.WriteString("\n\n")
		}
	}
	b.WriteString("\n【当前任务】生成：")
	b.WriteString(step)
	b.WriteString("\n")
	b.WriteString("【执行提醒】严格按 system 中 YAML 的 output 与 constraints 生成；hard=true 约束必须全部满足。\n")
	return b.String()
}

func buildSectionRepairPrompt(step string, issues []string) string {
	label := step
	switch step {
	case "title":
		label = "title"
	case "bullets":
		label = "bullet points"
	case "description":
		label = "product description"
	case "search_terms":
		label = "search terms"
	}
	return fmt.Sprintf(
		"上一版%s存在以下问题，必须逐条修复：\n- %s\n请基于上一版内容直接修复，并且只输出修复后的最终文本，不要解释。",
		label,
		strings.Join(issues, "\n- "),
	)
}

func buildJSONRepairPrompt(issueText, schema string) string {
	return fmt.Sprintf(
		"上一版输出不符合要求：%s\n请基于上一版内容直接修复。必须只返回一个合法 JSON object，格式示例：%s。禁止输出 JSON 以外的任何内容。",
		strings.TrimSpace(issueText),
		schema,
	)
}

func resolveModelForAttempt(opts sectionGenerateOptions, attempt int) (string, bool) {
	model := strings.TrimSpace(opts.ProviderCfg.Model)
	if model == "" {
		model = "deepseek-chat"
	}
	if !strings.EqualFold(opts.Provider, "deepseek") {
		return model, false
	}
	fb := opts.ProviderCfg.ThinkingFallback
	if !fb.Enabled {
		return model, false
	}
	if fb.Attempt <= 0 {
		fb.Attempt = 3
	}
	if attempt < fb.Attempt {
		return model, false
	}
	fallbackModel := strings.TrimSpace(fb.Model)
	if fallbackModel == "" {
		fallbackModel = "deepseek-reasoner"
	}
	if strings.EqualFold(model, fallbackModel) {
		return model, false
	}
	return fallbackModel, true
}

func validateSectionText(step, lang string, req listing.Requirement, text string, rule config.SectionRuleFile, tolerance int) ([]string, []string) {
	issues := make([]string, 0)
	warnings := make([]string, 0)
	switch step {
	case "title":
		t := cleanTitleLine(text)
		if t == "" {
			issues = append(issues, "标题为空")
			return issues, warnings
		}
		max := rule.Parsed.Constraints.MaxChars.Value
		n := runeLen(t)
		bounds := resolveCharBounds(0, max, tolerance)
		if bounds.hasRule() && !bounds.inRule(n) {
			if bounds.inTolerance(n) {
				warnings = append(warnings, fmt.Sprintf("标题长度 %d 未落入规则区间 %s，但落入容差区间 %s，已放行", n, bounds.ruleText(), bounds.toleranceText()))
			} else {
				issues = append(issues, fmt.Sprintf("标题长度超出容差区间：%d 不在 %s（规则区间 %s）", n, bounds.toleranceText(), bounds.ruleText()))
			}
		}
		if lang == "en" {
			topN := rule.Parsed.Constraints.MustContainTopNKeywords.Value
			if topN > len(req.Keywords) {
				topN = len(req.Keywords)
			}
			for i := 0; i < topN; i++ {
				kw := strings.TrimSpace(req.Keywords[i])
				if kw == "" {
					continue
				}
				if !strings.Contains(strings.ToLower(t), strings.ToLower(kw)) {
					issues = append(issues, fmt.Sprintf("标题缺少关键词 #%d: %s", i+1, kw))
				}
			}
		}
	case "bullets":
		expected := rule.Parsed.Output.Lines
		items, err := parseBullets(text, expected)
		if err != nil {
			issues = append(issues, err.Error())
			return issues, warnings
		}
		for i, it := range items {
			n := runeLen(it)
			minLen := rule.Parsed.Constraints.MinCharsPerLine.Value
			maxLen := rule.Parsed.Constraints.MaxCharsPerLine.Value
			bounds := resolveCharBounds(minLen, maxLen, tolerance)
			if bounds.hasRule() && !bounds.inRule(n) {
				if bounds.inTolerance(n) {
					warnings = append(warnings, fmt.Sprintf("第%d点长度 %d 未落入规则区间 %s，但落入容差区间 %s，已放行", i+1, n, bounds.ruleText(), bounds.toleranceText()))
				} else {
					issues = append(issues, fmt.Sprintf("第%d点长度超出容差区间：%d 不在 %s（规则区间 %s）", i+1, n, bounds.toleranceText(), bounds.ruleText()))
				}
			}
		}
	case "description":
		expected := rule.Parsed.Output.Paragraphs
		pars, err := parseParagraphs(text, expected)
		if err != nil {
			issues = append(issues, err.Error())
			return issues, warnings
		}
		for i, p := range pars {
			if strings.TrimSpace(p) == "" {
				issues = append(issues, fmt.Sprintf("描述第%d段为空", i+1))
			}
		}
	case "search_terms":
		line := cleanSearchTermsLine(text)
		if line == "" {
			issues = append(issues, "搜索词为空")
			return issues, warnings
		}
		if countNonEmptyLines(text) != rule.Parsed.Output.Lines {
			issues = append(issues, fmt.Sprintf("搜索词行数错误：%d != %d", countNonEmptyLines(text), rule.Parsed.Output.Lines))
		}
		max := rule.Parsed.Constraints.MaxChars.Value
		n := runeLen(line)
		bounds := resolveCharBounds(0, max, tolerance)
		if bounds.hasRule() && !bounds.inRule(n) {
			if bounds.inTolerance(n) {
				warnings = append(warnings, fmt.Sprintf("搜索词长度 %d 未落入规则区间 %s，但落入容差区间 %s，已放行", n, bounds.ruleText(), bounds.toleranceText()))
			} else {
				issues = append(issues, fmt.Sprintf("搜索词长度超出容差区间：%d 不在 %s（规则区间 %s）", n, bounds.toleranceText(), bounds.ruleText()))
			}
		}
	}
	return dedupeIssues(issues), dedupeIssues(warnings)
}

type charBounds struct {
	ruleMin int
	ruleMax int
	tolMin  int
	tolMax  int
	hasMin  bool
	hasMax  bool
}

func resolveCharBounds(minConstraint, maxConstraint, tolerance int) charBounds {
	if tolerance < 0 {
		tolerance = 0
	}
	minC := minConstraint
	maxC := maxConstraint
	if minC > 0 && maxC > 0 && minC > maxC {
		minC, maxC = maxC, minC
	}

	out := charBounds{
		ruleMin: minC,
		ruleMax: maxC,
		tolMin:  minC,
		tolMax:  maxC,
		hasMin:  minC > 0,
		hasMax:  maxC > 0,
	}

	switch {
	case minC > 0 && maxC > 0:
		out.tolMin = minC - tolerance
		if out.tolMin < 0 {
			out.tolMin = 0
		}
		out.tolMax = maxC + tolerance
	case maxC > 0:
		out.hasMin = false
		out.tolMin = 0
		out.tolMax = maxC + tolerance
	case minC > 0:
		out.tolMin = minC - tolerance
		if out.tolMin < 0 {
			out.tolMin = 0
		}
		out.hasMax = false
		out.tolMax = 0
	default:
		out.hasMin = false
		out.hasMax = false
		out.tolMin = 0
		out.tolMax = 0
	}
	return out
}

func (b charBounds) hasRule() bool {
	return b.hasMin || b.hasMax
}

func (b charBounds) inRule(n int) bool {
	if !b.hasRule() {
		return true
	}
	if b.hasMin && n < b.ruleMin {
		return false
	}
	if b.hasMax && n > b.ruleMax {
		return false
	}
	return true
}

func (b charBounds) inTolerance(n int) bool {
	if !b.hasRule() {
		return true
	}
	if b.hasMin && n < b.tolMin {
		return false
	}
	if b.hasMax && n > b.tolMax {
		return false
	}
	return true
}

func (b charBounds) ruleText() string {
	return formatRangeText(b.ruleMin, b.ruleMax, b.hasMin, b.hasMax)
}

func (b charBounds) toleranceText() string {
	return formatRangeText(b.tolMin, b.tolMax, b.hasMin, b.hasMax)
}

func formatRangeText(minV, maxV int, hasMin, hasMax bool) string {
	switch {
	case hasMin && hasMax:
		return fmt.Sprintf("[%d,%d]", minV, maxV)
	case hasMin:
		return fmt.Sprintf("[%d,+inf)", minV)
	case hasMax:
		return fmt.Sprintf("(-inf,%d]", maxV)
	default:
		return "(-inf,+inf)"
	}
}

var bulletPrefixRe = regexp.MustCompile(`^\s*(?:[-*•]|[0-9]{1,2}[\.)])\s*`)
var inlineBulletMarkerRe = regexp.MustCompile(`(?:^|\s)(?:[-*•]|[0-9]{1,2}[\.)])\s*`)

func parseBullets(text string, expected int) ([]string, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, expected)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = bulletPrefixRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	if len(out) != expected {
		if len(out) == 1 {
			inline := splitInlineBullets(out[0])
			if len(inline) == expected {
				return inline, nil
			}
			semi := splitBySeparator(out[0], ";")
			if len(semi) == expected {
				return semi, nil
			}
		}
		return nil, fmt.Errorf("五点数量错误：%d != %d", len(out), expected)
	}
	return out, nil
}

func splitInlineBullets(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	locs := inlineBulletMarkerRe.FindAllStringIndex(line, -1)
	if len(locs) < 2 {
		return nil
	}
	items := make([]string, 0, len(locs))
	for i, loc := range locs {
		start := loc[1]
		end := len(line)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		part := strings.TrimSpace(line[start:end])
		part = bulletPrefixRe.ReplaceAllString(part, "")
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func splitBySeparator(line, sep string) []string {
	if !strings.Contains(line, sep) {
		return nil
	}
	parts := strings.Split(line, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = bulletPrefixRe.ReplaceAllString(p, "")
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseParagraphs(text string, expected int) ([]string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	chunks := splitByBlankLines(text)
	if len(chunks) != expected {
		return nil, fmt.Errorf("描述段落数量错误：%d != %d", len(chunks), expected)
	}
	return chunks, nil
}

func splitByBlankLines(text string) []string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, 4)
	buf := make([]string, 0, 16)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		out = append(out, strings.TrimSpace(strings.Join(buf, " ")))
		buf = buf[:0]
	}
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			flush()
			continue
		}
		buf = append(buf, strings.TrimSpace(ln))
	}
	flush()
	return out
}

func countNonEmptyLines(text string) int {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	n := 0
	for _, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}

func normalizeModelText(text string) string {
	t := strings.TrimSpace(text)
	t = strings.ReplaceAll(t, "\r\n", "\n")
	t = strings.ReplaceAll(t, "\r", "\n")
	t = strings.ReplaceAll(t, "\u2028", "\n")
	t = strings.ReplaceAll(t, "\u2029", "\n")
	// Some providers return literal escaped newlines ("\n") in one line.
	// Convert them back so downstream line/paragraph validators can work.
	if !strings.Contains(t, "\n") && strings.Contains(t, `\n`) {
		t = strings.ReplaceAll(t, `\n`, "\n")
	}
	t = strings.ReplaceAll(t, "<br/>", "\n")
	t = strings.ReplaceAll(t, "<br />", "\n")
	t = strings.ReplaceAll(t, "<br>", "\n")
	if strings.HasPrefix(t, "```") {
		t = strings.TrimPrefix(t, "```")
		t = strings.TrimSpace(t)
		if strings.HasPrefix(strings.ToLower(t), "text") {
			t = strings.TrimSpace(t[4:])
		}
		if strings.HasPrefix(strings.ToLower(t), "markdown") {
			t = strings.TrimSpace(t[8:])
		}
		if i := strings.LastIndex(t, "```"); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
	}
	return t
}

func normalizeSingleLine(text string) string {
	text = normalizeModelText(text)
	lines := strings.Split(text, "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			return ln
		}
	}
	return ""
}

var lineLabelPrefixRe = regexp.MustCompile(`(?i)^(title|标题|search\s*terms|搜索词)\s*[:：]\s*`)
var categoryLabelPrefixRe = regexp.MustCompile(`(?i)^(category|分类)\s*[:：]\s*`)
var keywordLabelPrefixRe = regexp.MustCompile(`(?i)^(keywords?|关键词)\s*[:：]\s*`)

func cleanTitleLine(text string) string {
	line := normalizeSingleLine(text)
	line = bulletPrefixRe.ReplaceAllString(line, "")
	line = lineLabelPrefixRe.ReplaceAllString(line, "")
	return strings.TrimSpace(line)
}

func cleanSearchTermsLine(text string) string {
	line := normalizeSingleLine(text)
	line = lineLabelPrefixRe.ReplaceAllString(line, "")
	return strings.TrimSpace(line)
}

func cleanBulletLine(text string) string {
	line := normalizeSingleLine(text)
	line = bulletPrefixRe.ReplaceAllString(line, "")
	return strings.TrimSpace(line)
}

func trimToMaxByWords(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || runeLen(s) <= maxRunes {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		rs := []rune(s)
		if len(rs) > maxRunes {
			return strings.TrimSpace(string(rs[:maxRunes]))
		}
		return s
	}
	out := ""
	for _, w := range words {
		candidate := w
		if out != "" {
			candidate = out + " " + w
		}
		if runeLen(candidate) > maxRunes {
			break
		}
		out = candidate
	}
	if strings.TrimSpace(out) == "" {
		rs := []rune(s)
		if len(rs) > maxRunes {
			out = string(rs[:maxRunes])
		} else {
			out = s
		}
	}
	return strings.TrimSpace(out)
}

func padToMinByKeywords(s string, minRunes, maxRunes int, keywords []string) string {
	s = strings.TrimSpace(s)
	if minRunes <= 0 || runeLen(s) >= minRunes {
		return s
	}
	addition := func(base, tail string) string {
		if strings.TrimSpace(tail) == "" {
			return base
		}
		candidate := strings.TrimSpace(base + " " + strings.TrimSpace(tail))
		if maxRunes > 0 && runeLen(candidate) > maxRunes {
			return base
		}
		return candidate
	}
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(strings.ToLower(s), strings.ToLower(kw)) {
			continue
		}
		before := s
		s = addition(s, kw)
		if s == before {
			continue
		}
		if runeLen(s) >= minRunes {
			return s
		}
	}
	fallbacks := []string{
		"for everyday use",
		"for classroom decoration",
		"for party hanging display",
	}
	for _, f := range fallbacks {
		before := s
		s = addition(s, f)
		if s == before {
			continue
		}
		if runeLen(s) >= minRunes {
			return s
		}
	}
	return s
}

func cleanCategoryLine(text string) string {
	line := normalizeSingleLine(text)
	line = categoryLabelPrefixRe.ReplaceAllString(line, "")
	return strings.TrimSpace(line)
}

func cleanKeywordLine(text string) string {
	line := normalizeSingleLine(text)
	line = keywordLabelPrefixRe.ReplaceAllString(line, "")
	line = bulletPrefixRe.ReplaceAllString(line, "")
	return strings.TrimSpace(line)
}

func validateDocumentBySectionRules(lang string, req listing.Requirement, doc ListingDocument, rules config.SectionRules) error {
	if strings.TrimSpace(doc.Category) == "" {
		return fmt.Errorf("category 为空")
	}
	if lang == "en" && strings.TrimSpace(doc.Category) != strings.TrimSpace(req.Category) {
		return fmt.Errorf("category 与输入不一致")
	}
	if len(doc.Keywords) == 0 {
		return fmt.Errorf("keywords 为空")
	}
	if lang == "en" && len(doc.Keywords) != len(req.Keywords) {
		return fmt.Errorf("keywords 数量与输入不一致")
	}
	if lang == "cn" && len(doc.Keywords) != len(req.Keywords) {
		return fmt.Errorf("cn keywords 数量错误：%d != %d", len(doc.Keywords), len(req.Keywords))
	}
	if lang == "cn" {
		for i, kw := range doc.Keywords {
			if strings.TrimSpace(kw) == "" {
				return fmt.Errorf("cn keywords 第%d项为空", i+1)
			}
		}
	}
	if len(doc.BulletPoints) != rules.BulletCount() {
		return fmt.Errorf("五点数量错误")
	}
	if len(doc.DescriptionParagraphs) != rules.DescriptionParagraphs() {
		return fmt.Errorf("描述段落数量错误")
	}
	return nil
}
