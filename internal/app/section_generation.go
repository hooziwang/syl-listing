package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/logging"
)

type sectionGenerateOptions struct {
	Req         listing.Requirement
	Lang        string
	Provider    string
	ProviderCfg config.ProviderConfig
	APIKey      string
	Rules       config.SectionRules
	MaxRetries  int
	Client      *llm.Client
	Logger      *logging.Logger
	Candidate   int
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
		items, itemLatency, itemErr := generateBulletsLineByLine(opts, doc, bulletRule)
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
		userPrompt := buildSectionUserPrompt(step, opts.Req, doc)
		if lastIssues != "" {
			userPrompt += "\n\n【上次输出问题，必须全部修复】\n" + lastIssues
		}

		reqEvent := logging.Event{
			Event:     "api_request_" + step,
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      opts.Lang,
			Provider:  opts.Provider,
			Model:     opts.ProviderCfg.Model,
			APIMode:   opts.ProviderCfg.APIMode,
			BaseURL:   opts.ProviderCfg.BaseURL,
			Attempt:   attempt,
		}
		if opts.Logger.Verbose() {
			reqEvent.SystemPrompt = systemPrompt
			reqEvent.UserPrompt = userPrompt
		}
		opts.Logger.Emit(reqEvent)
		resp, err := opts.Client.Generate(context.Background(), llm.Request{
			Provider:        opts.Provider,
			BaseURL:         opts.ProviderCfg.BaseURL,
			Model:           opts.ProviderCfg.Model,
			APIMode:         opts.ProviderCfg.APIMode,
			APIKey:          opts.APIKey,
			ReasoningEffort: opts.ProviderCfg.ModelReasoningEffort,
			SystemPrompt:    systemPrompt,
			UserPrompt:      userPrompt,
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
			Model:     opts.ProviderCfg.Model,
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
		issues := validateSectionText(step, opts.Lang, opts.Req, text, sectionRule)
		if len(issues) > 0 {
			lastIssues = "- " + strings.Join(issues, "\n- ")
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

func generateBulletsLineByLine(opts sectionGenerateOptions, doc ListingDocument, rule config.SectionRuleFile) ([]string, int64, error) {
	expected := rule.Parsed.Output.Lines
	if expected <= 0 {
		return nil, 0, fmt.Errorf("bullets 规则 output.lines 无效：%d", expected)
	}
	minLen := rule.Parsed.Constraints.MinCharsPerLine.Value
	maxLen := rule.Parsed.Constraints.MaxCharsPerLine.Value
	out := make([]string, 0, expected)
	var total int64

	for idx := 1; idx <= expected; idx++ {
		var (
			lineOut   string
			latencyMS int64
		)
		lastIssues := ""
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
			systemPrompt := buildSectionSystemPrompt(rule)
			tmpDoc := doc
			tmpDoc.BulletPoints = append([]string{}, out...)
			userPrompt := buildSectionUserPrompt("bullets", opts.Req, tmpDoc)
			userPrompt += fmt.Sprintf("【子任务】只生成第%d条五点。只输出一行英文文本，不要编号，不要前缀。", idx)
			if lastIssues != "" {
				userPrompt += "\n\n【上次输出问题，必须全部修复】\n" + lastIssues
			}

			reqEvent := logging.Event{
				Event:     fmt.Sprintf("api_request_bullets_item_%d", idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Provider:  opts.Provider,
				Model:     opts.ProviderCfg.Model,
				APIMode:   opts.ProviderCfg.APIMode,
				BaseURL:   opts.ProviderCfg.BaseURL,
				Attempt:   attempt,
			}
			if opts.Logger.Verbose() {
				reqEvent.SystemPrompt = systemPrompt
				reqEvent.UserPrompt = userPrompt
			}
			opts.Logger.Emit(reqEvent)
			resp, err := opts.Client.Generate(context.Background(), llm.Request{
				Provider:        opts.Provider,
				BaseURL:         opts.ProviderCfg.BaseURL,
				Model:           opts.ProviderCfg.Model,
				APIMode:         opts.ProviderCfg.APIMode,
				APIKey:          opts.APIKey,
				ReasoningEffort: opts.ProviderCfg.ModelReasoningEffort,
				SystemPrompt:    systemPrompt,
				UserPrompt:      userPrompt,
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

			raw := normalizeModelText(resp.Text)
			respEvent := logging.Event{
				Event:     fmt.Sprintf("api_response_bullets_item_%d", idx),
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      opts.Lang,
				Provider:  opts.Provider,
				Model:     opts.ProviderCfg.Model,
				LatencyMS: resp.LatencyMS,
				Attempt:   attempt,
			}
			if opts.Logger.Verbose() {
				respEvent.ResponseText = raw
			}
			opts.Logger.Emit(respEvent)
			issues := make([]string, 0, 4)
			if countNonEmptyLines(raw) != 1 {
				issues = append(issues, fmt.Sprintf("第%d条应为单行输出", idx))
			}
			line := cleanBulletLine(raw)
			if strings.TrimSpace(line) == "" {
				issues = append(issues, fmt.Sprintf("第%d条为空", idx))
			}
			n := runeLen(line)
			if minLen > 0 && n < minLen {
				issues = append(issues, fmt.Sprintf("第%d条太短：%d < %d", idx, n, minLen))
			}
			if maxLen > 0 && n > maxLen {
				issues = append(issues, fmt.Sprintf("第%d条太长：%d > %d", idx, n, maxLen))
			}
			if len(issues) > 0 {
				lastIssues = "- " + strings.Join(issues, "\n- ")
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
			lineOut = line
			latencyMS = resp.LatencyMS
			return nil
		})
		if err != nil {
			if strings.TrimSpace(lastIssues) == "" {
				lastIssues = err.Error()
			}
			return nil, total, fmt.Errorf("bullets 第%d条重试后仍失败: %s", idx, lastIssues)
		}
		total += latencyMS
		out = append(out, lineOut)
	}
	return out, total, nil
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

func validateSectionText(step, lang string, req listing.Requirement, text string, rule config.SectionRuleFile) []string {
	issues := make([]string, 0)
	switch step {
	case "title":
		t := cleanTitleLine(text)
		if t == "" {
			issues = append(issues, "标题为空")
			return issues
		}
		max := rule.Parsed.Constraints.MaxChars.Value
		if runeLen(t) > max {
			issues = append(issues, fmt.Sprintf("标题超长：%d > %d", runeLen(t), max))
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
			return issues
		}
		for i, it := range items {
			n := runeLen(it)
			minLen := rule.Parsed.Constraints.MinCharsPerLine.Value
			maxLen := rule.Parsed.Constraints.MaxCharsPerLine.Value
			if minLen > 0 && n < minLen {
				issues = append(issues, fmt.Sprintf("第%d点太短：%d < %d", i+1, n, minLen))
			}
			if maxLen > 0 && n > maxLen {
				issues = append(issues, fmt.Sprintf("第%d点太长：%d > %d", i+1, n, maxLen))
			}
		}
	case "description":
		expected := rule.Parsed.Output.Paragraphs
		pars, err := parseParagraphs(text, expected)
		if err != nil {
			issues = append(issues, err.Error())
			return issues
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
			return issues
		}
		if countNonEmptyLines(text) != rule.Parsed.Output.Lines {
			issues = append(issues, fmt.Sprintf("搜索词行数错误：%d != %d", countNonEmptyLines(text), rule.Parsed.Output.Lines))
		}
		max := rule.Parsed.Constraints.MaxChars.Value
		if max > 0 && runeLen(line) > max {
			issues = append(issues, fmt.Sprintf("搜索词超长：%d > %d", runeLen(line), max))
		}
	}
	return dedupeIssues(issues)
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
