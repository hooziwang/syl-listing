package config

import "testing"

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	if cfg.Provider != "deepseek" {
		t.Fatalf("provider: %s", cfg.Provider)
	}
	if cfg.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Fatalf("api_key_env: %s", cfg.APIKeyEnv)
	}
	if cfg.RulesCenter.Owner == "" || cfg.RulesCenter.Repo == "" {
		t.Fatalf("rules center defaults missing")
	}
	if cfg.Output.Dir != "." || cfg.Output.Num != 1 {
		t.Fatalf("output defaults mismatch: %+v", cfg.Output)
	}
	ds, ok := cfg.Providers["deepseek"]
	if !ok {
		t.Fatalf("deepseek provider missing")
	}
	if ds.ThinkingFallback.Attempt != 3 || ds.ThinkingFallback.Model == "" {
		t.Fatalf("fallback defaults mismatch: %+v", ds.ThinkingFallback)
	}
}

func TestApplyDefaultsProviderForcedDeepseek(t *testing.T) {
	cfg := &Config{Provider: "openai", Providers: map[string]ProviderConfig{"deepseek": {}}}
	cfg.applyDefaults()
	if cfg.Provider != "deepseek" {
		t.Fatalf("expected deepseek, got %s", cfg.Provider)
	}
}
