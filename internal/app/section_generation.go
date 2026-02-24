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
	Generation  config.GenerationConfig
	APIKey      string
	RulesText   string
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

	bulletsText, latency, err := generateSectionWithRetry(opts, "bullets", doc)
	total += latency
	if err != nil {
		return ListingDocument{}, total, err
	}
	bullets, err := parseBullets(bulletsText, opts.Generation.BulletCount)
	if err != nil {
		return ListingDocument{}, total, err
	}
	doc.BulletPoints = bullets

	descText, latency, err := generateSectionWithRetry(opts, "description", doc)
	total += latency
	if err != nil {
		return ListingDocument{}, total, err
	}
	desc, err := parseParagraphs(descText, opts.Generation.DescriptionParagraphs)
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

	return doc, total, validateDocumentBySectionRules(opts.Lang, opts.Req, doc, opts.Generation)
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
		systemPrompt := buildSectionSystemPrompt(sectionRulesText(opts, step), opts.Lang, step, opts.Generation)
		userPrompt := buildSectionUserPrompt(step, opts.Req, doc)
		if lastIssues != "" {
			userPrompt += "\n\n【上次输出问题，必须全部修复】\n" + lastIssues
		}

		opts.Logger.Emit(logging.Event{Event: "api_request_" + step, Input: opts.Req.SourcePath, Candidate: opts.Candidate, Lang: opts.Lang, Provider: opts.Provider, Model: opts.ProviderCfg.Model, Attempt: attempt})
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
		issues := validateSectionText(step, opts.Lang, opts.Req, text, opts.Generation)
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

func sectionRulesText(opts sectionGenerateOptions, step string) string {
	switch step {
	case "title":
		if strings.TrimSpace(opts.Rules.Title) != "" {
			return opts.Rules.Title
		}
	case "bullets":
		if strings.TrimSpace(opts.Rules.Bullets) != "" {
			return opts.Rules.Bullets
		}
	case "description":
		if strings.TrimSpace(opts.Rules.Description) != "" {
			return opts.Rules.Description
		}
	case "search_terms":
		if strings.TrimSpace(opts.Rules.SearchTerms) != "" {
			return opts.Rules.SearchTerms
		}
	}
	return opts.RulesText
}

func buildSectionSystemPrompt(rules, lang, step string, gen config.GenerationConfig) string {
	common := rules + "\n\n【程序硬约束】\n" +
		"1) 只输出纯文本，不要 JSON，不要 Markdown 标题，不要解释\n" +
		"2) 不要输出代码块\n" +
		"3) 只输出英文\n"

	switch step {
	case "title":
		return common + fmt.Sprintf("4) 仅输出一行标题\n5) 标题最大 %d 字符\n", gen.TitleMaxEN)
	case "bullets":
		return common + fmt.Sprintf("4) 仅输出 %d 行，每行一个五点内容，不要编号和符号前缀\n5) 每行长度 %d-%d 字符\n", gen.BulletCount, gen.BulletMinChars, gen.BulletMaxChars)
	case "description":
		return common + fmt.Sprintf("4) 仅输出 %d 段，段与段之间空一行\n", gen.DescriptionParagraphs)
	case "search_terms":
		return common + fmt.Sprintf("4) 仅输出一行搜索词\n5) 最大 %d 字符\n", gen.SearchTermsMaxChars)
	default:
		return common
	}
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
	return b.String()
}

func validateSectionText(step, lang string, req listing.Requirement, text string, gen config.GenerationConfig) []string {
	issues := make([]string, 0)
	switch step {
	case "title":
		t := cleanTitleLine(text)
		if t == "" {
			issues = append(issues, "标题为空")
			return issues
		}
		max := gen.TitleMaxEN
		if lang == "cn" {
			max = gen.TitleMaxCN
		}
		if runeLen(t) > max {
			issues = append(issues, fmt.Sprintf("标题超长：%d > %d", runeLen(t), max))
		}
		if lang == "en" {
			topN := gen.TitleMustContainTopNKW
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
		items, err := parseBullets(text, gen.BulletCount)
		if err != nil {
			issues = append(issues, err.Error())
			return issues
		}
		for i, it := range items {
			n := runeLen(it)
			if n < gen.BulletMinChars {
				issues = append(issues, fmt.Sprintf("第%d点太短：%d < %d", i+1, n, gen.BulletMinChars))
			}
			if n > gen.BulletMaxChars {
				issues = append(issues, fmt.Sprintf("第%d点太长：%d > %d", i+1, n, gen.BulletMaxChars))
			}
		}
	case "description":
		pars, err := parseParagraphs(text, gen.DescriptionParagraphs)
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
		if gen.SearchTermsMaxChars > 0 && runeLen(line) > gen.SearchTermsMaxChars {
			issues = append(issues, fmt.Sprintf("搜索词超长：%d > %d", runeLen(line), gen.SearchTermsMaxChars))
		}
	}
	return dedupeIssues(issues)
}

var bulletPrefixRe = regexp.MustCompile(`^\s*(?:[-*•]|[0-9]{1,2}[\.)])\s*`)

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
		return nil, fmt.Errorf("五点数量错误：%d != %d", len(out), expected)
	}
	return out, nil
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

func normalizeModelText(text string) string {
	t := strings.TrimSpace(text)
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

func validateDocumentBySectionRules(lang string, req listing.Requirement, doc ListingDocument, gen config.GenerationConfig) error {
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
	if len(doc.BulletPoints) != gen.BulletCount {
		return fmt.Errorf("五点数量错误")
	}
	if len(doc.DescriptionParagraphs) != gen.DescriptionParagraphs {
		return fmt.Errorf("描述段落数量错误")
	}
	return nil
}
