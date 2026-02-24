package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/logging"
	"syl-listing/internal/translator"
)

type bilingualGenerateOptions struct {
	Req             listing.Requirement
	Provider        string
	ProviderCfg     config.ProviderConfig
	Generation      config.GenerationConfig
	Translation     config.TranslationConfig
	APIKey          string
	TranslateSID    string
	TranslateSK     string
	Rules           config.SectionRules
	MaxRetries      int
	Client          *llm.Client
	TranslateClient *translator.Client
	Logger          *logging.Logger
	Candidate       int
}

func generateENAndTranslateCNBySections(opts bilingualGenerateOptions) (ListingDocument, ListingDocument, int64, int64, error) {
	enDoc := ListingDocument{
		Keywords: append([]string{}, opts.Req.Keywords...),
		Category: strings.TrimSpace(opts.Req.Category),
	}
	cnDoc := ListingDocument{
		Keywords: make([]string, 0, len(opts.Req.Keywords)),
		Category: "",
	}
	var enLatencyTotal int64
	var cnLatencyTotal atomic.Int64

	cnBullets := make([]string, opts.Generation.BulletCount)
	cnDesc := make([]string, opts.Generation.DescriptionParagraphs)
	cnKeywords := make([]string, len(opts.Req.Keywords))
	var (
		translateWG  sync.WaitGroup
		translateMu  sync.Mutex
		translateErr error
		errOnce      sync.Once
	)
	defer translateWG.Wait()

	recordTranslateErr := func(err error) {
		errOnce.Do(func() {
			translateMu.Lock()
			translateErr = err
			translateMu.Unlock()
		})
	}
	scheduleTranslate := func(section, sourceText string, onSuccess func(string)) {
		translateWG.Add(1)
		go func() {
			defer translateWG.Done()
			translated, latency, err := translateSectionWithRetry(translateSectionOptions{
				Req:         opts.Req,
				Section:     section,
				SourceText:  sourceText,
				Generation:  opts.Generation,
				Translation: opts.Translation,
				SecretID:    opts.TranslateSID,
				SecretKey:   opts.TranslateSK,
				MaxRetries:  opts.MaxRetries,
				Client:      opts.TranslateClient,
				Logger:      opts.Logger,
				Candidate:   opts.Candidate,
			})
			if err != nil {
				recordTranslateErr(err)
				return
			}
			cnLatencyTotal.Add(latency)
			onSuccess(strings.TrimSpace(translated))
		}()
	}
	scheduleTranslateBatch := func(section string, sourceTexts []string, onSuccess func([]string)) {
		translateWG.Add(1)
		go func() {
			defer translateWG.Done()
			translated, latency, err := translateSectionsBatchWithRetry(translateBatchOptions{
				Req:         opts.Req,
				Section:     section,
				SourceTexts: sourceTexts,
				Translation: opts.Translation,
				SecretID:    opts.TranslateSID,
				SecretKey:   opts.TranslateSK,
				MaxRetries:  opts.MaxRetries,
				Client:      opts.TranslateClient,
				Logger:      opts.Logger,
				Candidate:   opts.Candidate,
			})
			if err != nil {
				recordTranslateErr(err)
				return
			}
			cnLatencyTotal.Add(latency)
			onSuccess(translated)
		}()
	}

	scheduleTranslate("category", strings.TrimSpace(opts.Req.Category), func(v string) {
		cnDoc.Category = cleanCategoryLine(v)
	})
	if isBatchTranslationProvider(opts.Translation.Provider) {
		scheduleTranslateBatch("keywords", opts.Req.Keywords, func(items []string) {
			for i := range cnKeywords {
				cnKeywords[i] = cleanKeywordLine(items[i])
			}
		})
	} else {
		for i, kw := range opts.Req.Keywords {
			idx := i
			src := kw
			scheduleTranslate(fmt.Sprintf("keyword_%d", i+1), src, func(v string) {
				cnKeywords[idx] = cleanKeywordLine(v)
			})
		}
	}

	enSectionOpts := sectionGenerateOptions{
		Req:         opts.Req,
		Lang:        "en",
		Provider:    opts.Provider,
		ProviderCfg: opts.ProviderCfg,
		Generation:  opts.Generation,
		APIKey:      opts.APIKey,
		Rules:       opts.Rules,
		MaxRetries:  opts.MaxRetries,
		Client:      opts.Client,
		Logger:      opts.Logger,
		Candidate:   opts.Candidate,
	}

	title, latency, err := generateSectionWithRetry(enSectionOpts, "title", enDoc)
	enLatencyTotal += latency
	if err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	enDoc.Title = cleanTitleLine(title)
	scheduleTranslate("title", enDoc.Title, func(v string) { cnDoc.Title = cleanTitleLine(v) })

	bulletsText, latency, err := generateSectionWithRetry(enSectionOpts, "bullets", enDoc)
	enLatencyTotal += latency
	if err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	enBullets, err := parseBullets(bulletsText, opts.Generation.BulletCount)
	if err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	enDoc.BulletPoints = enBullets
	if isBatchTranslationProvider(opts.Translation.Provider) {
		scheduleTranslateBatch("bullets", enBullets, func(items []string) {
			for i := range cnBullets {
				cnBullets[i] = strings.TrimSpace(items[i])
			}
		})
	} else {
		for i, bp := range enBullets {
			idx := i
			text := bp
			scheduleTranslate(fmt.Sprintf("bullet_%d", i+1), text, func(v string) {
				cnBullets[idx] = strings.TrimSpace(v)
			})
		}
	}

	descText, latency, err := generateSectionWithRetry(enSectionOpts, "description", enDoc)
	enLatencyTotal += latency
	if err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	enDesc, err := parseParagraphs(descText, opts.Generation.DescriptionParagraphs)
	if err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	enDoc.DescriptionParagraphs = enDesc
	if isBatchTranslationProvider(opts.Translation.Provider) {
		scheduleTranslateBatch("description", enDesc, func(items []string) {
			for i := range cnDesc {
				cnDesc[i] = strings.TrimSpace(items[i])
			}
		})
	} else {
		for i, p := range enDesc {
			idx := i
			text := p
			scheduleTranslate(fmt.Sprintf("description_%d", i+1), text, func(v string) {
				cnDesc[idx] = strings.TrimSpace(v)
			})
		}
	}

	search, latency, err := generateSectionWithRetry(enSectionOpts, "search_terms", enDoc)
	enLatencyTotal += latency
	if err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	enDoc.SearchTerms = cleanSearchTermsLine(search)
	scheduleTranslate("search_terms", enDoc.SearchTerms, func(v string) { cnDoc.SearchTerms = cleanSearchTermsLine(v) })

	translateWG.Wait()
	if translateErr != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), translateErr
	}
	cnDoc.Keywords = cnKeywords
	cnDoc.BulletPoints = cnBullets
	cnDoc.DescriptionParagraphs = cnDesc
	for i, bp := range cnDoc.BulletPoints {
		if strings.TrimSpace(bp) == "" {
			return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), fmt.Errorf("cn bullets 校验失败：第%d点为空", i+1)
		}
	}
	for i, p := range cnDoc.DescriptionParagraphs {
		if strings.TrimSpace(p) == "" {
			return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), fmt.Errorf("cn description 校验失败：第%d段为空", i+1)
		}
	}
	if strings.TrimSpace(cnDoc.Title) == "" {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), fmt.Errorf("cn title 校验失败：为空")
	}
	if strings.TrimSpace(cnDoc.Category) == "" {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), fmt.Errorf("cn category 校验失败：为空")
	}
	for i, kw := range cnDoc.Keywords {
		if strings.TrimSpace(kw) == "" {
			return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), fmt.Errorf("cn keywords 校验失败：第%d项为空", i+1)
		}
	}
	if strings.TrimSpace(cnDoc.SearchTerms) == "" {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), fmt.Errorf("cn search_terms 校验失败：为空")
	}

	if err := validateDocumentBySectionRules("en", opts.Req, enDoc, opts.Generation); err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	if err := validateDocumentBySectionRules("cn", opts.Req, cnDoc, opts.Generation); err != nil {
		return ListingDocument{}, ListingDocument{}, enLatencyTotal, cnLatencyTotal.Load(), err
	}
	return enDoc, cnDoc, enLatencyTotal, cnLatencyTotal.Load(), nil
}

type translateSectionOptions struct {
	Req         listing.Requirement
	Section     string
	SourceText  string
	Generation  config.GenerationConfig
	Translation config.TranslationConfig
	SecretID    string
	SecretKey   string
	MaxRetries  int
	Client      *translator.Client
	Logger      *logging.Logger
	Candidate   int
}

type translateBatchOptions struct {
	Req         listing.Requirement
	Section     string
	SourceTexts []string
	Translation config.TranslationConfig
	SecretID    string
	SecretKey   string
	MaxRetries  int
	Client      *translator.Client
	Logger      *logging.Logger
	Candidate   int
}

func translateSectionWithRetry(opts translateSectionOptions) (string, int64, error) {
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
				Event:     "retry_backoff_translate_" + opts.Section,
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      "cn",
				Attempt:   attempt,
				WaitMS:    wait.Milliseconds(),
				Error:     err.Error(),
			})
		},
	}, func(attempt int) error {
		opts.Logger.Emit(logging.Event{Event: "api_request_translate_" + opts.Section, Input: opts.Req.SourcePath, Candidate: opts.Candidate, Lang: "cn", Attempt: attempt})
		resp, err := opts.Client.Translate(context.Background(), translator.Request{
			Provider:   opts.Translation.Provider,
			Endpoint:   opts.Translation.BaseURL,
			Model:      opts.Translation.Model,
			SecretID:   opts.SecretID,
			SecretKey:  opts.SecretKey,
			Region:     opts.Translation.Region,
			Source:     opts.Translation.Source,
			Target:     opts.Translation.Target,
			ProjectID:  opts.Translation.ProjectID,
			UserPrompt: opts.SourceText,
		})
		if err != nil {
			lastIssues = "- 翻译请求失败: " + err.Error()
			opts.Logger.Emit(logging.Event{Level: "warn", Event: "api_error_translate_" + opts.Section, Input: opts.Req.SourcePath, Candidate: opts.Candidate, Lang: "cn", Attempt: attempt, Error: err.Error()})
			return errors.New(lastIssues)
		}
		text := normalizeModelText(resp.Text)
		if strings.TrimSpace(text) == "" {
			lastIssues = "- 翻译结果为空"
			opts.Logger.Emit(logging.Event{Level: "warn", Event: "validate_error_translate_" + opts.Section, Input: opts.Req.SourcePath, Candidate: opts.Candidate, Lang: "cn", Attempt: attempt, Error: "翻译结果为空"})
			return errors.New(lastIssues)
		}
		outText = text
		outLatency = resp.LatencyMS
		return nil
	})
	if err != nil {
		if strings.TrimSpace(lastIssues) == "" {
			lastIssues = err.Error()
		}
		return "", 0, fmt.Errorf("%s 翻译重试后仍失败：%s", opts.Section, lastIssues)
	}
	return outText, outLatency, nil
}

func translateSectionsBatchWithRetry(opts translateBatchOptions) ([]string, int64, error) {
	var (
		outTexts   []string
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
				Event:     "retry_backoff_translate_" + opts.Section + "_batch",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      "cn",
				Attempt:   attempt,
				WaitMS:    wait.Milliseconds(),
				Error:     err.Error(),
			})
		},
	}, func(attempt int) error {
		opts.Logger.Emit(logging.Event{
			Event:     "api_request_translate_" + opts.Section + "_batch",
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      "cn",
			Attempt:   attempt,
		})
		resp, err := opts.Client.TranslateBatch(context.Background(), translator.Request{
			Provider:  opts.Translation.Provider,
			Endpoint:  opts.Translation.BaseURL,
			Model:     opts.Translation.Model,
			SecretID:  opts.SecretID,
			SecretKey: opts.SecretKey,
			Region:    opts.Translation.Region,
			Source:    opts.Translation.Source,
			Target:    opts.Translation.Target,
			ProjectID: opts.Translation.ProjectID,
		}, opts.SourceTexts)
		if err != nil {
			lastIssues = "- 批量翻译请求失败: " + err.Error()
			opts.Logger.Emit(logging.Event{
				Level:     "warn",
				Event:     "api_error_translate_" + opts.Section + "_batch",
				Input:     opts.Req.SourcePath,
				Candidate: opts.Candidate,
				Lang:      "cn",
				Attempt:   attempt,
				Error:     err.Error(),
			})
			return errors.New(lastIssues)
		}
		if len(resp.Texts) != len(opts.SourceTexts) {
			lastIssues = fmt.Sprintf("- 批量翻译返回数量不匹配：%d != %d", len(resp.Texts), len(opts.SourceTexts))
			return errors.New(lastIssues)
		}
		for i, text := range resp.Texts {
			if strings.TrimSpace(text) == "" {
				lastIssues = fmt.Sprintf("- 批量翻译第%d项为空", i+1)
				return errors.New(lastIssues)
			}
		}
		outTexts = make([]string, len(resp.Texts))
		copy(outTexts, resp.Texts)
		outLatency = resp.LatencyMS
		return nil
	})
	if err != nil {
		if strings.TrimSpace(lastIssues) == "" {
			lastIssues = err.Error()
		}
		return nil, 0, fmt.Errorf("%s 批量翻译重试后仍失败：%s", opts.Section, lastIssues)
	}
	return outTexts, outLatency, nil
}

func isBatchTranslationProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "tencent", "tencent_tmt":
		return true
	default:
		return false
	}
}
