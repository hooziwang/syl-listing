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
	CWD             string
	Stdout          io.Writer
	Stderr          io.Writer
	InvokedSubcmd   bool
	NormalizedInput []string
}

type Result struct {
	Succeeded int
	Failed    int
}

type candidateJob struct {
	Req       listing.Requirement
	Candidate int
}

func Run(opts Options) (Result, error) {
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

	logger, closer, err := logging.New(opts.Stdout, opts.LogFile)
	if err != nil {
		return Result{}, fmt.Errorf("初始化日志失败：%w", err)
	}
	if closer != nil {
		defer closer.Close()
	}

	logger.Emit(logging.Event{Event: "startup", Provider: cfg.Provider, Model: providerCfg.Model})
	logger.Emit(logging.Event{Event: "config_loaded", Input: paths.ConfigSource})

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
	result := Result{}
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

	var wg sync.WaitGroup

	go func() {
		for _, req := range validReqs {
			for i := 1; i <= cfg.Output.Num; i++ {
				job := candidateJob{Req: req, Candidate: i}
				wg.Add(1)
				go func(j candidateJob) {
					defer wg.Done()
					ok := processCandidate(processCandidateOptions{
						Job:         j,
						OutDir:      outDir,
						Provider:    cfg.Provider,
						ProviderCfg: providerCfg,
						Generation:  cfg.Generation,
						APIKey:      apiKey,
						Rules:       rules,
						MaxRetries:  cfg.MaxRetries,
						Client:      client,
						Logger:      logger,
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
	logger.Emit(logging.Event{Event: "finished", Attempt: result.Succeeded + result.Failed, Error: fmt.Sprintf("success=%d failed=%d", result.Succeeded, result.Failed)})
	return result, nil
}

type processCandidateOptions struct {
	Job         candidateJob
	OutDir      string
	Provider    string
	ProviderCfg config.ProviderConfig
	Generation  config.GenerationConfig
	APIKey      string
	Rules       config.SectionRules
	MaxRetries  int
	Client      *llm.Client
	Logger      *logging.Logger
}

func processCandidate(opts processCandidateOptions) bool {
	id, enPath, err := output.NextEN(opts.OutDir, 8, nil)
	if err != nil {
		opts.Logger.Emit(logging.Event{Level: "error", Event: "name_failed", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Error: err.Error()})
		return false
	}
	_ = id

	enDoc, enLatency, err := generateDocumentBySections(sectionGenerateOptions{
		Req:         opts.Job.Req,
		Lang:        "en",
		Provider:    opts.Provider,
		ProviderCfg: opts.ProviderCfg,
		Generation:  opts.Generation,
		APIKey:      opts.APIKey,
		Rules:       opts.Rules,
		MaxRetries:  opts.MaxRetries,
		Client:      opts.Client,
		Logger:      opts.Logger,
		Candidate:   opts.Job.Candidate,
	})
	if err != nil {
		opts.Logger.Emit(logging.Event{Level: "error", Event: "generate_failed", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Error: err.Error()})
		return false
	}
	opts.Logger.Emit(logging.Event{Event: "generate_ok", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "en", LatencyMS: enLatency})

	enMD := RenderMarkdown("en", opts.Job.Req, enDoc)
	if err := os.WriteFile(enPath, []byte(enMD), 0o644); err != nil {
		opts.Logger.Emit(logging.Event{Level: "error", Event: "write_failed", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "en", OutputFile: enPath, Error: err.Error()})
		return false
	}
	opts.Logger.Emit(logging.Event{Event: "write_ok", Input: opts.Job.Req.SourcePath, Candidate: opts.Job.Candidate, Lang: "en", OutputFile: enPath})
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
