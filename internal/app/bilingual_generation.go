package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/logging"
	"syl-listing/internal/translator"
)

type bilingualGenerateOptions struct {
	Req                  listing.Requirement
	CharTolerance        int
	Provider             string
	ProviderCfg          config.ProviderConfig
	TranslateProviderCfg config.ProviderConfig
	APIKey               string
	Rules                config.SectionRules
	MaxRetries           int
	Client               *llm.Client
	TranslateClient      *translator.Client
	Logger               *logging.Logger
	Candidate            int
}

func generateENAndTranslateCNBySections(opts bilingualGenerateOptions) (ListingDocument, ListingDocument, int64, int64, error) {
	startAt := time.Now()
	var enElapsedMS int64
	enDoc := ListingDocument{
		Keywords: append([]string{}, opts.Req.Keywords...),
		Category: strings.TrimSpace(opts.Req.Category),
	}
	cnDoc := ListingDocument{
		Keywords: make([]string, 0, len(opts.Req.Keywords)),
		Category: "",
	}

	cnBullets := make([]string, opts.Rules.BulletCount())
	cnDesc := make([]string, opts.Rules.DescriptionParagraphs())
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
			translated, _, err := translateSectionWithRetry(translateSectionOptions{
				Req:                  opts.Req,
				Section:              section,
				SourceText:           sourceText,
				TranslateProviderCfg: opts.TranslateProviderCfg,
				APIKey:               opts.APIKey,
				MaxRetries:           opts.MaxRetries,
				Client:               opts.TranslateClient,
				Logger:               opts.Logger,
				Candidate:            opts.Candidate,
			})
			if err != nil {
				recordTranslateErr(err)
				return
			}
			onSuccess(strings.TrimSpace(translated))
		}()
	}
	scheduleTranslate("category", strings.TrimSpace(opts.Req.Category), func(v string) {
		cnDoc.Category = cleanCategoryLine(v)
	})
	for i, kw := range opts.Req.Keywords {
		idx := i
		src := kw
		scheduleTranslate(fmt.Sprintf("keyword_%d", i+1), src, func(v string) {
			cnKeywords[idx] = cleanKeywordLine(v)
		})
	}

	enSectionOpts := sectionGenerateOptions{
		Req:           opts.Req,
		Lang:          "en",
		CharTolerance: opts.CharTolerance,
		Provider:      opts.Provider,
		ProviderCfg:   opts.ProviderCfg,
		APIKey:        opts.APIKey,
		Rules:         opts.Rules,
		MaxRetries:    opts.MaxRetries,
		Client:        opts.Client,
		Logger:        opts.Logger,
		Candidate:     opts.Candidate,
	}

	title, latency, err := generateSectionWithRetry(enSectionOpts, "title", enDoc)
	_ = latency
	if err != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, err
	}
	enDoc.Title = cleanTitleLine(title)
	scheduleTranslate("title", enDoc.Title, func(v string) { cnDoc.Title = cleanTitleLine(v) })

	bulletRule, err := opts.Rules.Get("bullets")
	if err != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, err
	}
	var enBullets []string
	if strings.EqualFold(opts.Provider, "deepseek") {
		items, itemLatency, itemErr := generateBulletsWithJSONAndRepair(enSectionOpts, enDoc, bulletRule)
		_ = itemLatency
		if itemErr != nil {
			return ListingDocument{}, ListingDocument{}, 0, 0, itemErr
		}
		enBullets = items
	} else {
		bulletsText, sectionLatency, sectionErr := generateSectionWithRetry(enSectionOpts, "bullets", enDoc)
		_ = sectionLatency
		if sectionErr != nil {
			return ListingDocument{}, ListingDocument{}, 0, 0, sectionErr
		}
		items, parseErr := parseBullets(bulletsText, opts.Rules.BulletCount())
		if parseErr != nil {
			return ListingDocument{}, ListingDocument{}, 0, 0, parseErr
		}
		enBullets = items
	}
	enDoc.BulletPoints = enBullets
	for i, bp := range enBullets {
		idx := i
		text := bp
		scheduleTranslate(fmt.Sprintf("bullet_%d", i+1), text, func(v string) {
			cnBullets[idx] = strings.TrimSpace(v)
		})
	}

	descText, latency, err := generateSectionWithRetry(enSectionOpts, "description", enDoc)
	_ = latency
	if err != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, err
	}
	enDesc, err := parseParagraphs(descText, opts.Rules.DescriptionParagraphs())
	if err != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, err
	}
	enDoc.DescriptionParagraphs = enDesc
	for i, p := range enDesc {
		idx := i
		text := p
		scheduleTranslate(fmt.Sprintf("description_%d", i+1), text, func(v string) {
			cnDesc[idx] = strings.TrimSpace(v)
		})
	}

	search, latency, err := generateSectionWithRetry(enSectionOpts, "search_terms", enDoc)
	_ = latency
	if err != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, err
	}
	enDoc.SearchTerms = cleanSearchTermsLine(search)
	enElapsedMS = time.Since(startAt).Milliseconds()
	scheduleTranslate("search_terms", enDoc.SearchTerms, func(v string) { cnDoc.SearchTerms = cleanSearchTermsLine(v) })

	translateWG.Wait()
	if translateErr != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, translateErr
	}
	cnDoc.Keywords = cnKeywords
	cnDoc.BulletPoints = cnBullets
	cnDoc.DescriptionParagraphs = cnDesc
	for i, bp := range cnDoc.BulletPoints {
		if strings.TrimSpace(bp) == "" {
			return ListingDocument{}, ListingDocument{}, 0, 0, fmt.Errorf("cn bullets 校验失败：第%d点为空", i+1)
		}
	}
	for i, p := range cnDoc.DescriptionParagraphs {
		if strings.TrimSpace(p) == "" {
			return ListingDocument{}, ListingDocument{}, 0, 0, fmt.Errorf("cn description 校验失败：第%d段为空", i+1)
		}
	}
	if strings.TrimSpace(cnDoc.Title) == "" {
		return ListingDocument{}, ListingDocument{}, 0, 0, fmt.Errorf("cn title 校验失败：为空")
	}
	if strings.TrimSpace(cnDoc.Category) == "" {
		return ListingDocument{}, ListingDocument{}, 0, 0, fmt.Errorf("cn category 校验失败：为空")
	}
	for i, kw := range cnDoc.Keywords {
		if strings.TrimSpace(kw) == "" {
			return ListingDocument{}, ListingDocument{}, 0, 0, fmt.Errorf("cn keywords 校验失败：第%d项为空", i+1)
		}
	}
	if strings.TrimSpace(cnDoc.SearchTerms) == "" {
		return ListingDocument{}, ListingDocument{}, 0, 0, fmt.Errorf("cn search_terms 校验失败：为空")
	}

	if err := validateDocumentBySectionRules("en", opts.Req, enDoc, opts.Rules); err != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, err
	}
	if err := validateDocumentBySectionRules("cn", opts.Req, cnDoc, opts.Rules); err != nil {
		return ListingDocument{}, ListingDocument{}, 0, 0, err
	}
	return enDoc, cnDoc, enElapsedMS, time.Since(startAt).Milliseconds(), nil
}

type translateSectionOptions struct {
	Req                  listing.Requirement
	Section              string
	SourceText           string
	TranslateProviderCfg config.ProviderConfig
	APIKey               string
	MaxRetries           int
	Client               *translator.Client
	Logger               *logging.Logger
	Candidate            int
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
		reqEvent := logging.Event{
			Event:     "api_request_translate_" + opts.Section,
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      "cn",
			Provider:  "deepseek",
			Model:     opts.TranslateProviderCfg.Model,
			BaseURL:   opts.TranslateProviderCfg.BaseURL,
			Attempt:   attempt,
		}
		if opts.Logger.Verbose() {
			reqEvent.SourceText = opts.SourceText
		}
		opts.Logger.Emit(reqEvent)
		resp, err := opts.Client.Translate(context.Background(), translator.Request{
			Provider:   "deepseek",
			Endpoint:   opts.TranslateProviderCfg.BaseURL,
			Model:      opts.TranslateProviderCfg.Model,
			APIKey:     opts.APIKey,
			Source:     "en",
			Target:     "zh",
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
		respEvent := logging.Event{
			Event:     "api_response_translate_" + opts.Section,
			Input:     opts.Req.SourcePath,
			Candidate: opts.Candidate,
			Lang:      "cn",
			Provider:  "deepseek",
			Model:     opts.TranslateProviderCfg.Model,
			LatencyMS: resp.LatencyMS,
			Attempt:   attempt,
		}
		if opts.Logger.Verbose() {
			respEvent.ResponseText = text
		}
		opts.Logger.Emit(respEvent)
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
