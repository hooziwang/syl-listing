package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"syl-listing/internal/config"
	"syl-listing/internal/discovery"
	"syl-listing/internal/listing"
	"syl-listing/internal/llm"
	"syl-listing/internal/logging"
	"syl-listing/internal/output"
	"syl-listing/internal/translator"
)

type Options struct {
	Inputs          []string
	ConfigPath      string
	OutputDir       string
	Num             int
	Concurrency     int
	MaxRetries      int
	Provider        string
	LogFile         string
	Verbose         bool
	CWD             string
	Stdout          io.Writer
	Stderr          io.Writer
	InvokedSubcmd   bool
	NormalizedInput []string
}

type Result struct {
	Succeeded int
	Failed    int
	ElapsedMS int64
	Balance   string
}

type candidateJob struct {
	Req       listing.Requirement
	Candidate int
}

func Run(opts Options) (result Result, err error) {
	runStartedAt := time.Now()
	cwd := strings.TrimSpace(opts.CWD)
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Result{}, fmt.Errorf("读取当前目录失败：%w", err)
		}
		cwd = wd
	}

	cfg, paths, err := config.Load(opts.ConfigPath, cwd)
	if err != nil {
		return Result{}, err
	}
	overrideConfig(cfg, opts)

	providerCfg, ok := cfg.Providers[cfg.Provider]
	if !ok {
		return Result{}, fmt.Errorf("配置中不存在 provider：%s", cfg.Provider)
	}
	translateProviderCfg, ok := cfg.Providers["deepseek"]
	if !ok {
		return Result{}, fmt.Errorf("配置中不存在 provider：deepseek（中文翻译固定使用 providers.deepseek）")
	}

	logger, closer, err := logging.New(opts.Stdout, opts.LogFile, opts.Verbose, cfg.Output.Num > 1)
	if err != nil {
		return Result{}, fmt.Errorf("初始化日志失败：%w", err)
	}
	if closer != nil {
		defer closer.Close()
	}
	logger.Emit(logging.Event{Event: "startup", Provider: cfg.Provider, Model: providerCfg.Model})
	logger.Emit(logging.Event{Event: "config_loaded", Input: paths.ConfigSource})

	syncRes, syncErr := config.SyncRulesFromCenter(cfg, paths)
	if syncErr != nil {
		return Result{}, syncErr
	}
	if strings.TrimSpace(syncRes.Warning) != "" {
		logger.Emit(logging.Event{Level: "warn", Event: "rules_sync_warning", Error: syncRes.Warning})
	}
	if syncRes.Updated && strings.TrimSpace(syncRes.Message) != "" {
		logger.Emit(logging.Event{Event: "rules_sync_updated", Error: syncRes.Message})
	}

	rules, err := config.ReadSectionRules(paths.ResolvedRulesDir)
	if err != nil {
		return Result{}, err
	}

	envMap, err := config.LoadEnvFile(paths.EnvPath)
	if err != nil {
		return Result{}, fmt.Errorf("未读取到 %s。先复制 %s 为 %s 并填写 %s", paths.EnvPath, paths.EnvExample, paths.EnvPath, cfg.APIKeyEnv)
	}
	apiKey := strings.TrimSpace(envMap[cfg.APIKeyEnv])
	if apiKey == "" {
		return Result{}, fmt.Errorf("%s 为空。先复制 %s 为 %s 并填写 key", cfg.APIKeyEnv, paths.EnvExample, paths.EnvPath)
	}
	balanceAPIKey := resolveDeepSeekBalanceKey(envMap, apiKey)
	defer func() {
		balance, fetchErr := fetchDeepSeekBalanceWithRetry(balanceAPIKey, cfg.MaxRetries)
		if fetchErr != nil {
			result.Balance = "查询失败"
			logger.Emit(logging.Event{Level: "warn", Event: "balance_failed", Error: fetchErr.Error()})
			return
		}
		result.Balance = formatBalanceForSummary(balance)
		logger.Emit(logging.Event{Event: "balance", Balance: balance})
	}()

	inputPaths := make([]string, 0, len(opts.Inputs))
	for _, in := range opts.Inputs {
		inputPaths = append(inputPaths, absPath(cwd, in))
	}
	discoverRes, err := discovery.Discover(inputPaths)
	if err != nil {
		return Result{}, err
	}
	for _, w := range discoverRes.Warnings {
		logger.Emit(logging.Event{Level: "warn", Event: "scan_warning", Error: w})
	}

	validReqs := make([]listing.Requirement, 0, len(discoverRes.Files))
	for _, file := range discoverRes.Files {
		req, parseErr := listing.ParseFile(file)
		if parseErr != nil {
			result.Failed++
			logger.Emit(logging.Event{Level: "error", Event: "parse_failed", Input: file, Error: parseErr.Error()})
			continue
		}
		if strings.TrimSpace(req.Brand) == "" {
			result.Failed++
			logger.Emit(logging.Event{Level: "error", Event: "validation_failed", Input: file, Error: "品牌名缺失"})
			continue
		}
		if strings.TrimSpace(req.Category) == "" {
			result.Failed++
			logger.Emit(logging.Event{Level: "error", Event: "validation_failed", Input: file, Error: "分类缺失"})
			continue
		}
		for _, w := range req.Warnings {
			logger.Emit(logging.Event{Level: "warn", Event: "validation_warning", Input: file, Error: w})
		}
		validReqs = append(validReqs, req)
	}

	if len(validReqs) == 0 {
		if result.Failed > 0 {
			result.ElapsedMS = time.Since(runStartedAt).Milliseconds()
			return result, nil
		}
		return result, fmt.Errorf("没有可生成的需求文件")
	}

	outDir := cfg.Output.Dir
	if strings.TrimSpace(outDir) == "" {
		outDir = "."
	}
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(cwd, outDir)
	}
	if err := output.EnsureDir(outDir); err != nil {
		return result, fmt.Errorf("创建输出目录失败：%w", err)
	}

	results := make(chan bool, len(validReqs)*cfg.Output.Num)
	client := llm.NewClient(time.Duration(cfg.RequestTimeoutSec) * time.Second)
	translateClient := translator.NewClient(time.Duration(cfg.RequestTimeoutSec) * time.Second)

	var wg sync.WaitGroup

	go func() {
		for _, req := range validReqs {
			for i := 1; i <= cfg.Output.Num; i++ {
				job := candidateJob{Req: req, Candidate: i}
				wg.Add(1)
				go func(j candidateJob) {
					defer wg.Done()
					ok := processCandidate(processCandidateOptions{
						Job:                  j,
						OutDir:               outDir,
						CharTolerance:        cfg.CharTolerance,
						Provider:             cfg.Provider,
						ProviderCfg:          providerCfg,
						TranslateProviderCfg: translateProviderCfg,
						APIKey:               apiKey,
						Rules:                rules,
						MaxRetries:           cfg.MaxRetries,
						Client:               client,
						TranslateClient:      translateClient,
						Logger:               logger,
					})
					results <- ok
				}(job)
			}
		}
		wg.Wait()
		close(results)
	}()

	for ok := range results {
		if ok {
			result.Succeeded++
		} else {
			result.Failed++
		}
	}
	result.ElapsedMS = time.Since(runStartedAt).Milliseconds()
	logger.Emit(logging.Event{Event: "finished", Attempt: result.Succeeded + result.Failed, Error: fmt.Sprintf("success=%d failed=%d", result.Succeeded, result.Failed)})
	return result, nil
}

type processCandidateOptions struct {
	Job                  candidateJob
	OutDir               string
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
}

func processCandidate(opts processCandidateOptions) bool {
	id, enPath, cnPath, err := output.NextPair(opts.OutDir, 8, nil)
	if err != nil {
		opts.Logger.Emit(logging.Event{Level: "error", Event: "name_failed", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Error: err.Error()})
		return false
	}
	_ = id

	enDoc, cnDoc, enLatency, cnLatency, err := generateENAndTranslateCNBySections(bilingualGenerateOptions{
		Req:                  opts.Job.Req,
		CharTolerance:        opts.CharTolerance,
		Provider:             opts.Provider,
		ProviderCfg:          opts.ProviderCfg,
		TranslateProviderCfg: opts.TranslateProviderCfg,
		APIKey:               opts.APIKey,
		Rules:                opts.Rules,
		MaxRetries:           opts.MaxRetries,
		Client:               opts.Client,
		TranslateClient:      opts.TranslateClient,
		Logger:               opts.Logger,
		Candidate:            opts.Job.Candidate,
	})
	if err != nil {
		opts.Logger.Emit(logging.Event{Level: "error", Event: "generate_failed", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Error: err.Error()})
		return false
	}
	opts.Logger.Emit(logging.Event{Event: "generate_ok", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "en", LatencyMS: enLatency})
	opts.Logger.Emit(logging.Event{Event: "generate_ok", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "cn", LatencyMS: cnLatency})

	enMD := RenderMarkdown("en", opts.Job.Req, enDoc)
	if err := os.WriteFile(enPath, []byte(enMD), 0o644); err != nil {
		opts.Logger.Emit(logging.Event{Level: "error", Event: "write_failed", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "en", OutputFile: enPath, Error: err.Error()})
		return false
	}
	cnMD := RenderMarkdown("cn", opts.Job.Req, cnDoc)
	if err := os.WriteFile(cnPath, []byte(cnMD), 0o644); err != nil {
		opts.Logger.Emit(logging.Event{Level: "error", Event: "write_failed", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "cn", OutputFile: cnPath, Error: err.Error()})
		return false
	}
	opts.Logger.Emit(logging.Event{Event: "write_ok", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "en", OutputFile: enPath})
	opts.Logger.Emit(logging.Event{Event: "write_ok", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "cn", OutputFile: cnPath})
	return true
}

func overrideConfig(cfg *config.Config, opts Options) {
	if strings.TrimSpace(opts.OutputDir) != "" {
		cfg.Output.Dir = opts.OutputDir
	}
	if opts.Num > 0 {
		cfg.Output.Num = opts.Num
	}
	if opts.Concurrency > 0 {
		cfg.Concurrency = opts.Concurrency
	}
	if opts.MaxRetries > 0 {
		cfg.MaxRetries = opts.MaxRetries
	}
	if strings.TrimSpace(opts.Provider) != "" {
		cfg.Provider = opts.Provider
	}
}

func absPath(cwd, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}
