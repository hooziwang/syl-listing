package config

import "strings"

type Config struct {
	Provider          string                    `yaml:"provider"`
	APIKeyEnv         string                    `yaml:"api_key_env"`
	RulesDir          string                    `yaml:"rules_dir"`
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
	BaseURL              string `yaml:"base_url"`
	APIMode              string `yaml:"api_mode"`
	Model                string `yaml:"model"`
	ModelReasoningEffort string `yaml:"model_reasoning_effort"`
}

type Paths struct {
	HomeDir          string
	RootDir          string
	ConfigPath       string
	RulesDir         string
	EnvPath          string
	EnvExample       string
	ConfigSource     string
	ResolvedRulesDir string
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
		}
	}
	if strings.TrimSpace(c.Provider) != "deepseek" {
		c.Provider = "deepseek"
	}
}
