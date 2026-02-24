package config

import "strings"

type Config struct {
	Provider          string                    `yaml:"provider"`
	APIKeyEnv         string                    `yaml:"api_key_env"`
	RulesDir          string                    `yaml:"rules_dir"`
	RulesCenter       RulesCenterConfig         `yaml:"rules_center"`
	CharTolerance     int                       `yaml:"char_tolerance"`
	Concurrency       int                       `yaml:"concurrency"`
	MaxRetries        int                       `yaml:"max_retries"`
	RequestTimeoutSec int                       `yaml:"request_timeout_sec"`
	Output            OutputConfig              `yaml:"output"`
	Providers         map[string]ProviderConfig `yaml:"providers"`
}

type OutputConfig struct {
	Dir string `yaml:"dir"`
	Num int    `yaml:"num"`
}

type ProviderConfig struct {
	BaseURL              string                 `yaml:"base_url"`
	APIMode              string                 `yaml:"api_mode"`
	Model                string                 `yaml:"model"`
	ModelReasoningEffort string                 `yaml:"model_reasoning_effort"`
	ThinkingFallback     ThinkingFallbackConfig `yaml:"thinking_fallback"`
}

type ThinkingFallbackConfig struct {
	Enabled bool   `yaml:"enabled"`
	Attempt int    `yaml:"attempt"`
	Model   string `yaml:"model"`
}

type Paths struct {
	HomeDir          string
	RootDir          string
	ConfigPath       string
	RulesDir         string
	RulesLockPath    string
	EnvPath          string
	EnvExample       string
	ConfigSource     string
	ResolvedRulesDir string
}

type RulesCenterConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Owner      string `yaml:"owner"`
	Repo       string `yaml:"repo"`
	Release    string `yaml:"release"`
	Asset      string `yaml:"asset"`
	TimeoutSec int    `yaml:"timeout_sec"`
	Strict     bool   `yaml:"strict"`
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Provider) == "" {
		c.Provider = "deepseek"
	}
	if strings.TrimSpace(c.APIKeyEnv) == "" {
		c.APIKeyEnv = "DEEPSEEK_API_KEY"
	}
	if strings.TrimSpace(c.RulesDir) == "" {
		c.RulesDir = "~/.syl-listing/rules"
	}
	if strings.TrimSpace(c.RulesCenter.Owner) == "" {
		c.RulesCenter.Owner = "hooziwang"
	}
	if strings.TrimSpace(c.RulesCenter.Repo) == "" {
		c.RulesCenter.Repo = "syl-listing-rules"
	}
	if strings.TrimSpace(c.RulesCenter.Release) == "" {
		c.RulesCenter.Release = "latest"
	}
	if strings.TrimSpace(c.RulesCenter.Asset) == "" {
		c.RulesCenter.Asset = "rules-bundle.tar.gz"
	}
	if c.RulesCenter.TimeoutSec <= 0 {
		c.RulesCenter.TimeoutSec = 20
	}
	if c.CharTolerance <= 0 {
		c.CharTolerance = 20
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 0
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.RequestTimeoutSec <= 0 {
		c.RequestTimeoutSec = 300
	}
	if strings.TrimSpace(c.Output.Dir) == "" {
		c.Output.Dir = "."
	}
	if c.Output.Num <= 0 {
		c.Output.Num = 1
	}
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}
	if _, ok := c.Providers["deepseek"]; !ok {
		c.Providers["deepseek"] = ProviderConfig{
			BaseURL:              "https://api.deepseek.com",
			APIMode:              "chat",
			Model:                "deepseek-chat",
			ModelReasoningEffort: "",
			ThinkingFallback: ThinkingFallbackConfig{
				Enabled: true,
				Attempt: 3,
				Model:   "deepseek-reasoner",
			},
		}
	}
	ds := c.Providers["deepseek"]
	if ds.ThinkingFallback.Attempt <= 0 {
		ds.ThinkingFallback.Attempt = 3
	}
	if strings.TrimSpace(ds.ThinkingFallback.Model) == "" {
		ds.ThinkingFallback.Model = "deepseek-reasoner"
	}
	c.Providers["deepseek"] = ds
	if strings.TrimSpace(c.Provider) != "deepseek" {
		c.Provider = "deepseek"
	}
}
