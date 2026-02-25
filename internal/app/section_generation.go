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

type sectionExecutionPolicy struct {
	Protocol      string
	Granularity   string
	ItemJSONField string
}

func resolveSectionExecutionPolicy(rule config.SectionRuleFile) sectionExecutionPolicy {
	return sectionExecutionPolicy{
		Protocol:      strings.TrimSpace(strings.ToLower(rule.Parsed.Execution.Generation.Protocol)),
		Granularity:   strings.TrimSpace(strings.ToLower(rule.Parsed.Execution.Repair.Granularity)),
		ItemJSONField: strings.TrimSpace(rule.Parsed.Execution.Repair.ItemJSONField),
	}
}

func (p sectionExecutionPolicy) useJSONLines() bool {
	return p.Protocol == "json_lines"
}

func (p sectionExecutionPolicy) useItemRepair() bool {
	return p.Granularity == "item"
}

func providerSupportsJSONMode(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), "deepseek")
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
	bulletPolicy := resolveSectionExecutionPolicy(bulletRule)
	if bulletPolicy.useJSONLines() {
		if !providerSupportsJSONMode(opts.Provider) {
			return ListingDocument{}, total, fmt.Errorf("provider %s 不支持 json_lines 协议", opts.Provider)
		}
		items, itemLatency, itemErr := generateJSONLinesWithRepair(opts, "bullets", doc, bulletRule)
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
	lengthIssue := false
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
		reqModel, thinkingFallback := resolveModelForAttempt(opts, sectionRule, attempt, lengthIssue)
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
			lengthIssue = containsLengthError(issues)
			opts.Logger.Emit(logging.Event{Level: "warn", Event: "validate_error_" + step, Input: opts.Req.SourcePath, Candidate: opts.Candidate, Lang: opts.Lang, Attempt: attempt, Error: strings.Join(issues, "; ")})
			return errors.New(strings.Join(issues, "; "))
		}
		outText = text
		outLatency = resp.LatencyMS
		lengthIssue = false
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
	return generateJSONLinesWithRepair(opts, "bullets", doc, rule)
}

func generateJSONLinesWithRepair(opts sectionGenerateOptions, step string, doc ListingDocument, rule config.SectionRuleFile) ([]string, int64, error) {
	expected := rule.Parsed.Output.Lines
	if expected <= 0 {
		return nil, 0, fmt.Errorf("%s 规则 output.lines 无效：%d", step, expected)
	}
	policy := resolveSectionExecutionPolicy(rule)
	bounds := resolveCharBounds(
		rule.Parsed.Constraints.MinCharsPerLine.Value,
		rule.Parsed.Constraints.MaxCharsPerLine.Value,
		opts.CharTolerance,
	)
	var total int64

	out, latencyMS, err := generateJSONLinesBatchWithRetry(opts, step, doc, rule, bounds)
	total += latencyMS
	if err != nil {
		return nil, total, err
	}

	for i := range out {
		out[i] = normalizeLineByBounds(cleanBulletLine(out[i]), bounds, opts.Req.Keywords)
	}
	invalidIndexes, issues, warnings := validateLineSet(step, out, bounds)
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
		return nil, total, fmt.Errorf("%s 校验失败：%s", step, strings.Join(issues, "; "))
	}
	if len(invalidIndexes) == 0 {
		return out, total, nil
	}
	if !policy.useItemRepair() {
		return nil, total, fmt.Errorf("%s 校验失败：%s", step, strings.Join(issues, "; "))
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
			line, latency, itemErr := regenerateJSONLineItemWithRetry(opts, step, doc, rule, idx, snapshot, bounds, policy.ItemJSONField)
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

	_, finalIssues, _ := validateLineSet(step, repaired, bounds)
	if len(finalIssues) > 0 {
		return nil, total, fmt.Errorf("%s 修复后仍不满足规则：%s", step, strings.Join(finalIssues, "; "))
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
	return generateJSONLinesBatchWithRetry(opts, "bullets", doc, rule, bounds)
}

func generateJSONLinesBatchWithRetry(opts sectionGenerateOptions, step string, doc ListingDocument, rule config.SectionRuleFile, bounds charBounds) ([]string, int64, error) {
	expected := rule.Parsed.Output.Lines
	var (
		outItems   []string
		outLatency int64
	)
	lastIssues := ""
	lengthIssue := false
	systemPrompt := buildSectionSystemPrompt(rule) + `

【JSON协议】
Return valid json only.
必须只返回一个 json object，禁止 markdown 代码块、禁止解释。
对象中必须包含一个字符串数组字段，长度必须满足 output.lines。`
	baseUserPrompt := buildSectionUserPrompt(step, opts.Req, doc) +
		fmt.Sprintf("\n【输出要求】必须返回 json object，其中字符串数组字段长度必须恰好 %d。", expected)
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
			Event:     "api_request_" + step,
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			APIMode:   opts.ProviderCfg.APIMode,
			BaseURL:   opts.ProviderCfg.BaseURL,
			Attempt:   attempt,
		}
		reqModel, thinkingFallback := resolveModelForAttempt(opts, rule, attempt, lengthIssue)
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
			JSONMode:        true,
		})
		if err != nil {
			lastIssues = "- API 调用失败: " + err.Error()
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "api_error_" + step,
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

		items, parseErr := parseLinesFromJSON(text, expected)
		if parseErr != nil {
			parseErrText := parseErr.Error()
			if step == "bullets" {
				parseErrText = strings.ReplaceAll(parseErrText, "行数错误", "五点数量错误")
			}
			lastIssues = "- JSON 解析失败: " + parseErrText
			history = append(history,
				llm.Message{Role: "assistant", Content: text},
				llm.Message{Role: "user", Content: buildJSONRepairPrompt(parseErrText, `{"items":["..."]}`)},
			)
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "validate_error_" + step,
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     parseErrText,
			})
			lengthIssue = false
			return errors.New(lastIssues)
		}
		_, lineIssues, _ := validateLineSet(step, items, bounds)
		lengthIssue = containsLengthError(lineIssues)
		outItems = items
		outLatency = resp.LatencyMS
		return nil
	})
	if err != nil {
		if strings.TrimSpace(lastIssues) == "" {
			lastIssues = err.Error()
		}
		return nil, 0, fmt.Errorf("%s 批量生成重试后仍失败: %s", step, lastIssues)
	}
	return outItems, outLatency, nil
}

func regenerateBulletItemJSONWithRetry(opts sectionGenerateOptions, doc ListingDocument, rule config.SectionRuleFile, idx int, current []string, bounds charBounds) (string, int64, error) {
	return regenerateJSONLineItemWithRetry(opts, "bullets", doc, rule, idx, current, bounds, "bullet")
}

func regenerateJSONLineItemWithRetry(opts sectionGenerateOptions, step string, doc ListingDocument, rule config.SectionRuleFile, idx int, current []string, bounds charBounds, itemField string) (string, int64, error) {
	var (
		outLine    string
		outLatency int64
	)
	lastIssues := ""
	lengthIssue := false
	systemPrompt := buildSectionSystemPrompt(rule) + `

【JSON协议】
Return valid json only.
必须只返回一个 json object，且对象中必须包含一个字符串字段。`
	tmpDoc := doc
	switch step {
	case "bullets":
		tmpDoc.BulletPoints = append([]string{}, current...)
	}
	if strings.TrimSpace(itemField) == "" {
		itemField = "item"
	}
	baseUserPrompt := buildSectionUserPrompt(step, opts.Req, tmpDoc) +
		fmt.Sprintf(
			"\n【子任务】只修复第%d条，返回 json object，且仅包含一个字符串字段（键名=%s）。\n【硬约束】只返回一行文本，不得包含换行；文本长度必须落在规则区间 %s（容差区间 %s）。",
			idx,
			itemField,
			bounds.ruleText(),
			bounds.toleranceText(),
		)
	history := make([]llm.Message, 0, 8)
	err := withExponentialBackoff(retryOptions{
		MaxRetries: opts.MaxRetries,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   8 * time.Second,
		Jitter:     0.25,
		OnRetry: func(attempt int, wait time.Duration, err error) {
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     fmt.Sprintf("retry_backoff_%s_item_%d", step, idx),
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
			Event:     fmt.Sprintf("api_request_%s_item_%d", step, idx),
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			APIMode:   opts.ProviderCfg.APIMode,
			BaseURL:   opts.ProviderCfg.BaseURL,
			Attempt:   attempt,
		}
		reqModel, thinkingFallback := resolveModelForAttempt(opts, rule, attempt, lengthIssue)
		reqEvent.Model = reqModel
		if thinkingFallback {
			opts.Logger.Emit(logging.Event{
				Event:     fmt.Sprintf("thinking_fallback_%s_item_%d", step, idx),
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
				Event:     fmt.Sprintf("api_error_%s_item_%d", step, idx),
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
			Event:     fmt.Sprintf("api_response_%s_item_%d", step, idx),
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

		line, parseErr := parseLineItemFromJSON(text, itemField)
		if parseErr != nil {
			lastIssues = "- JSON 解析失败: " + parseErr.Error()
			history = append(history,
				llm.Message{Role: "assistant", Content: text},
				llm.Message{Role: "user", Content: buildJSONRepairPrompt(parseErr.Error(), fmt.Sprintf("{\"%s\":\"...\"}", itemField))},
			)
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     fmt.Sprintf("validate_error_%s_item_%d", step, idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     parseErr.Error(),
			})
			lengthIssue = false
			return errors.New(lastIssues)
		}
		line = normalizeLineByBounds(cleanBulletLine(line), bounds, opts.Req.Keywords)

		issues, warnings := validateLineItem(step, idx, line, bounds)
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
				llm.Message{Role: "user", Content: buildJSONRepairPrompt(strings.Join(issues, "; "), fmt.Sprintf("{\"%s\":\"...\"}", itemField))},
			)
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     fmt.Sprintf("validate_error_%s_item_%d", step, idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Attempt:   attempt,
				Error:     strings.Join(issues, "; "),
			})
			lengthIssue = containsLengthError(issues)
			return errors.New(strings.Join(issues, "; "))
		}
		outLine = cleanBulletLine(line)
		outLatency = resp.LatencyMS
		lengthIssue = false
		return nil
	})
	if err != nil {
		if strings.TrimSpace(lastIssues) == "" {
			lastIssues = err.Error()
		}
		return "", 0, fmt.Errorf("%s 第%d条修复重试后仍失败: %s", step, idx, lastIssues)
	}
	return outLine, outLatency, nil
}

func validateBulletSet(items []string, bounds charBounds) ([]int, []string, []string) {
	return validateLineSet("bullets", items, bounds)
}

func validateLineSet(step string, items []string, bounds charBounds) ([]int, []string, []string) {
	invalid := make([]int, 0, len(items))
	issues := make([]string, 0, len(items))
	warnings := make([]string, 0, len(items))
	for i, it := range items {
		lineIssues, lineWarnings := validateLineItem(step, i+1, it, bounds)
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
	return validateLineItem("bullets", idx, raw, bounds)
}

func validateLineItem(step string, idx int, raw string, bounds charBounds) ([]string, []string) {
	issues := make([]string, 0, 3)
	warnings := make([]string, 0, 1)
	itemLabel := fmt.Sprintf("第%d行", idx)
	if step == "bullets" {
		itemLabel = fmt.Sprintf("第%d条", idx)
	}
	if countNonEmptyLines(raw) != 1 {
		issues = append(issues, fmt.Sprintf("%s应为单行输出", itemLabel))
	}
	line := cleanBulletLine(raw)
	if strings.TrimSpace(line) == "" {
		issues = append(issues, fmt.Sprintf("%s为空", itemLabel))
		return issues, warnings
	}
	n := runeLen(line)
	if bounds.hasRule() && !bounds.inRule(n) {
		if bounds.inTolerance(n) {
			warnings = append(warnings, fmt.Sprintf("%s长度 %d 未落入规则区间 %s，但落入容差区间 %s，已放行", itemLabel, n, bounds.ruleText(), bounds.toleranceText()))
		} else {
			issues = append(issues, fmt.Sprintf("%s长度超出容差区间：%d 不在 %s（规则区间 %s）", itemLabel, n, bounds.toleranceText(), bounds.ruleText()))
		}
	}
	return issues, warnings
}

func parseBulletsFromJSON(text string, expected int) ([]string, error) {
	items, err := parseLinesFromJSON(text, expected)
	if err == nil {
		return items, nil
	}
	msg := strings.ReplaceAll(err.Error(), "行数错误", "五点数量错误")
	return nil, errors.New(msg)
}

func parseLinesFromJSON(text string, expected int) ([]string, error) {
	var payload bulletsBatchJSON
	if err := decodeJSONObject(text, &payload); err != nil {
		return nil, err
	}
	items := payload.Bullets
	if len(items) == 0 {
		items = payload.Items
	}
	if len(items) == 0 {
		if alt, err := extractStringArrayFromJSONObject(text); err == nil {
			items = alt
		}
	}
	if len(items) != expected {
		return nil, fmt.Errorf("行数错误：%d != %d", len(items), expected)
	}
	out := make([]string, 0, expected)
	for i, it := range items {
		clean := cleanBulletLine(it)
		if strings.TrimSpace(clean) == "" {
			return nil, fmt.Errorf("第%d行为空", i+1)
		}
		out = append(out, clean)
	}
	return out, nil
}

func parseBulletItemFromJSON(text string) (string, error) {
	return parseLineItemFromJSON(text, "bullet")
}

func parseLineItemFromJSON(text string, itemField string) (string, error) {
	var payload bulletItemJSON
	if err := decodeJSONObject(text, &payload); err != nil {
		return "", err
	}
	line := ""
	if strings.TrimSpace(itemField) != "" {
		if v, err := extractStringByKeyFromJSONObject(text, itemField); err == nil {
			line = strings.TrimSpace(v)
		}
	}
	if line == "" {
		line = strings.TrimSpace(payload.Bullet)
	}
	if line == "" {
		line = strings.TrimSpace(payload.Text)
	}
	if line == "" {
		line = strings.TrimSpace(payload.Item)
	}
	if line == "" {
		if alt, err := extractFirstStringOrArrayItemFromJSONObject(text); err == nil {
			line = strings.TrimSpace(alt)
		}
	}
	if line == "" {
		if strings.TrimSpace(itemField) != "" {
			return "", fmt.Errorf("缺少字符串字段（期望键名: %s）", itemField)
		}
		return "", fmt.Errorf("缺少字符串字段")
	}
	return line, nil
}

func extractStringByKeyFromJSONObject(text, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("key 为空")
	}
	raw := strings.TrimSpace(text)
	obj := extractJSONObject(raw)
	if strings.TrimSpace(obj) == "" {
		obj = raw
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(obj), &m); err != nil {
		return "", err
	}
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("未找到键: %s", key)
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil && strings.TrimSpace(s) != "" {
		return s, nil
	}
	var arr []string
	if err := json.Unmarshal(v, &arr); err == nil {
		for _, it := range arr {
			if strings.TrimSpace(it) != "" {
				return it, nil
			}
		}
	}
	return "", fmt.Errorf("键 %s 对应值不是非空字符串", key)
}

func extractStringArrayFromJSONObject(text string) ([]string, error) {
	raw := strings.TrimSpace(text)
	obj := extractJSONObject(raw)
	if strings.TrimSpace(obj) == "" {
		obj = raw
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(obj), &m); err != nil {
		return nil, err
	}
	for _, v := range m {
		var arr []string
		if err := json.Unmarshal(v, &arr); err == nil && len(arr) > 0 {
			return arr, nil
		}
	}
	return nil, fmt.Errorf("未找到字符串数组字段")
}

func extractFirstStringOrArrayItemFromJSONObject(text string) (string, error) {
	raw := strings.TrimSpace(text)
	obj := extractJSONObject(raw)
	if strings.TrimSpace(obj) == "" {
		obj = raw
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(obj), &m); err != nil {
		return "", err
	}
	for _, v := range m {
		var s string
		if err := json.Unmarshal(v, &s); err == nil && strings.TrimSpace(s) != "" {
			return s, nil
		}
		var arr []string
		if err := json.Unmarshal(v, &arr); err == nil {
			for _, it := range arr {
				if strings.TrimSpace(it) != "" {
					return it, nil
				}
			}
		}
	}
	return "", fmt.Errorf("未找到字符串字段")
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
	label := strings.ReplaceAll(strings.TrimSpace(step), "_", " ")
	if label == "" {
		label = "section"
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

func resolveModelForAttempt(opts sectionGenerateOptions, rule config.SectionRuleFile, attempt int, lengthIssue bool) (string, bool) {
	model := strings.TrimSpace(opts.ProviderCfg.Model)
	if model == "" {
		model = "deepseek-chat"
	}
	if !strings.EqualFold(opts.Provider, "deepseek") {
		return model, false
	}
	if lengthIssue && rule.Parsed.DisableThinkingFallbackOnLengthError() {
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

func containsLengthError(issues []string) bool {
	for _, s := range issues {
		t := strings.TrimSpace(strings.ToLower(s))
		if t == "" {
			continue
		}
		if strings.Contains(t, "长度") || strings.Contains(t, "length") || strings.Contains(t, "char") {
			return true
		}
	}
	return false
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

func normalizeLineByBounds(line string, bounds charBounds, keywords []string) string {
	line = strings.TrimSpace(line)
	if line == "" || !bounds.hasRule() {
		return line
	}
	if bounds.hasMax && bounds.tolMax > 0 && runeLen(line) > bounds.tolMax {
		line = trimToMaxByWords(line, bounds.tolMax)
	}
	if bounds.hasMin && bounds.tolMin > 0 && runeLen(line) < bounds.tolMin {
		line = padToMinByKeywords(line, bounds.tolMin, bounds.tolMax, keywords)
	}
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
