package config

import "strings"

type Config struct {
	Provider          string                    `yaml:"provider"`
	APIKeyEnv         string                    `yaml:"api_key_env"`
	RulesFile         string                    `yaml:"rules_file"`
	RulesDir          string                    `yaml:"rules_dir"`
	Concurrency       int                       `yaml:"concurrency"`
	MaxRetries        int                       `yaml:"max_retries"`
	RequestTimeoutSec int                       `yaml:"request_timeout_sec"`
	Output            OutputConfig              `yaml:"output"`
	Generation        GenerationConfig          `yaml:"generation"`
	Translation       TranslationConfig         `yaml:"translation"`
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

type GenerationConfig struct {
	TitleMaxEN             int `yaml:"title_max_en"`
	TitleMaxCN             int `yaml:"title_max_cn"`
	BulletCount            int `yaml:"bullet_count"`
	BulletMinChars         int `yaml:"bullet_min_chars"`
	BulletMaxChars         int `yaml:"bullet_max_chars"`
	DescriptionParagraphs  int `yaml:"description_paragraphs"`
	SearchTermsMaxChars    int `yaml:"search_terms_max_chars"`
	TitleMustContainTopNKW int `yaml:"title_must_contain_top_n_keywords"`
}

type TranslationConfig struct {
	Provider     string  `yaml:"provider"`
	BaseURL      string  `yaml:"base_url"`
	Model        string  `yaml:"model"`
	APIKeyEnv    string  `yaml:"api_key_env"`
	SecretIDEnv  string  `yaml:"secret_id_env"`
	SecretKeyEnv string  `yaml:"secret_key_env"`
	Region       string  `yaml:"region"`
	Source       string  `yaml:"source"`
	Target       string  `yaml:"target"`
	ProjectID    int64   `yaml:"project_id"`
	ThinkingType string  `yaml:"thinking_type"`
	Temperature  float64 `yaml:"temperature"`
	MaxTokens    int     `yaml:"max_tokens"`
}

type Paths struct {
	HomeDir          string
	RootDir          string
	ConfigPath       string
	RulesPath        string
	RulesDir         string
	EnvPath          string
	EnvExample       string
	ConfigSource     string
	ResolvedRules    string
	ResolvedRulesDir string
}

type SectionRules struct {
	Title       string
	Bullets     string
	Description string
	SearchTerms string
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Provider) == "" {
		c.Provider = "openai"
	}
	if strings.TrimSpace(c.APIKeyEnv) == "" {
		c.APIKeyEnv = "SYL_LISTING_API_KEY"
	}
	if strings.TrimSpace(c.RulesFile) == "" {
		c.RulesFile = "~/.syl-listing/rules.md"
	}
	if strings.TrimSpace(c.RulesDir) == "" {
		c.RulesDir = "~/.syl-listing/rules"
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
	if c.Generation.TitleMaxEN <= 0 {
		c.Generation.TitleMaxEN = 200
	}
	if c.Generation.TitleMaxCN <= 0 {
		c.Generation.TitleMaxCN = 100
	}
	if c.Generation.BulletCount <= 0 {
		c.Generation.BulletCount = 5
	}
	if c.Generation.BulletMinChars <= 0 {
		c.Generation.BulletMinChars = 235
	}
	if c.Generation.BulletMaxChars <= 0 {
		c.Generation.BulletMaxChars = 250
	}
	if c.Generation.DescriptionParagraphs <= 0 {
		c.Generation.DescriptionParagraphs = 2
	}
	if c.Generation.SearchTermsMaxChars <= 0 {
		c.Generation.SearchTermsMaxChars = 250
	}
	if c.Generation.TitleMustContainTopNKW <= 0 {
		c.Generation.TitleMustContainTopNKW = 3
	}
	if strings.TrimSpace(c.Translation.Provider) == "" {
		c.Translation.Provider = "tencent_tmt"
	}
	provider := strings.ToLower(strings.TrimSpace(c.Translation.Provider))
	baseURL := strings.TrimSpace(c.Translation.BaseURL)
	if provider == "tencent" || provider == "tencent_tmt" {
		if baseURL == "" {
			c.Translation.BaseURL = "https://tmt.tencentcloudapi.com"
		}
	} else {
		c.Translation.Provider = "tencent_tmt"
		if baseURL == "" {
			c.Translation.BaseURL = "https://tmt.tencentcloudapi.com"
		}
	}
	if strings.TrimSpace(c.Translation.Model) == "" {
		c.Translation.Model = "tmt"
	}
	if strings.TrimSpace(c.Translation.SecretIDEnv) == "" {
		c.Translation.SecretIDEnv = "TENCENTCLOUD_SECRET_ID"
	}
	if strings.TrimSpace(c.Translation.SecretKeyEnv) == "" {
		c.Translation.SecretKeyEnv = "TENCENTCLOUD_SECRET_KEY"
	}
	if strings.TrimSpace(c.Translation.Region) == "" {
		c.Translation.Region = "ap-beijing"
	}
	if strings.TrimSpace(c.Translation.Source) == "" {
		c.Translation.Source = "en"
	}
	if strings.TrimSpace(c.Translation.Target) == "" {
		c.Translation.Target = "zh"
	}
	if c.Translation.ProjectID < 0 {
		c.Translation.ProjectID = 0
	}
	if strings.TrimSpace(c.Translation.ThinkingType) == "" {
		c.Translation.ThinkingType = "disabled"
	}
	if c.Translation.Temperature == 0 {
		c.Translation.Temperature = 0.2
	}
	if c.Translation.MaxTokens <= 0 {
		c.Translation.MaxTokens = 1024
	}
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}

	if _, ok := c.Providers["openai"]; !ok {
		c.Providers["openai"] = ProviderConfig{
			BaseURL:              "https://flux-code.cc",
			APIMode:              "auto",
			Model:                "gpt-5.3-codex",
			ModelReasoningEffort: "high",
		}
	}
	if _, ok := c.Providers["gemini"]; !ok {
		c.Providers["gemini"] = ProviderConfig{
			BaseURL:              "https://generativelanguage.googleapis.com",
			APIMode:              "gemini",
			Model:                "gemini-2.5-pro",
			ModelReasoningEffort: "high",
		}
	}
	if _, ok := c.Providers["claude"]; !ok {
		c.Providers["claude"] = ProviderConfig{
			BaseURL:              "https://api.anthropic.com",
			APIMode:              "claude",
			Model:                "claude-sonnet-4-20250514",
			ModelReasoningEffort: "high",
		}
	}
}
