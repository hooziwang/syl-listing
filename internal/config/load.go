package config

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var embeddedDefaultConfig []byte

//go:embed default_env.example
var embeddedEnvExample []byte

//go:embed default_rules/*.md
var embeddedRuleFiles embed.FS

func Load(pathArg, cwd string) (*Config, *Paths, error) {
	paths, err := resolvePaths(pathArg)
	if err != nil {
		return nil, nil, err
	}
	if err := ensureBootstrap(paths); err != nil {
		return nil, nil, err
	}

	raw, err := os.ReadFile(paths.ConfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("读取配置文件失败（%s）：%w", paths.ConfigPath, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, nil, fmt.Errorf("配置文件格式错误（%s）：%w", paths.ConfigPath, err)
	}
	cfg.applyDefaults()

	paths.ConfigSource = paths.ConfigPath
	paths.ResolvedRules = expandPath(cfg.RulesFile, paths.HomeDir, cwd)
	paths.ResolvedRulesDir = expandPath(cfg.RulesDir, paths.HomeDir, cwd)
	if err := ensureRuleFiles(paths.ResolvedRulesDir); err != nil {
		return nil, nil, err
	}
	return cfg, paths, nil
}

func resolvePaths(configArg string) (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("读取用户目录失败：%w", err)
	}
	root := filepath.Join(home, ".syl-listing")
	configPath := filepath.Join(root, "config.yaml")
	if strings.TrimSpace(configArg) != "" {
		configPath = expandPath(configArg, home, "")
	}

	return &Paths{
		HomeDir:    home,
		RootDir:    root,
		ConfigPath: configPath,
		RulesPath:  filepath.Join(root, "rules.md"),
		RulesDir:   filepath.Join(root, "rules"),
		EnvPath:    filepath.Join(root, ".env"),
		EnvExample: filepath.Join(root, ".env.example"),
	}, nil
}

func ensureBootstrap(paths *Paths) error {
	if err := os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败：%w", err)
	}
	if err := ensureFile(paths.ConfigPath, embeddedDefaultConfig, 0o644); err != nil {
		return err
	}
	if err := ensureFile(paths.EnvExample, embeddedEnvExample, 0o644); err != nil {
		return err
	}
	if err := ensureRuleFiles(paths.RulesDir); err != nil {
		return err
	}
	return nil
}

func ensureFile(path string, data []byte, mode os.FileMode) error {
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return nil
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("写入默认文件失败（%s）：%w", path, err)
	}
	return nil
}

func expandPath(v, home, cwd string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if strings.HasPrefix(v, "~/") {
		return filepath.Join(home, v[2:])
	}
	if filepath.IsAbs(v) {
		return v
	}
	if strings.TrimSpace(cwd) != "" {
		return filepath.Join(cwd, v)
	}
	return v
}

func ReadRules(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取规则文件失败（%s）：%w", path, err)
	}
	return string(raw), nil
}

func ReadSectionRules(dir string) (SectionRules, error) {
	load := func(name string) (string, error) {
		p := filepath.Join(dir, name)
		raw, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("读取规则文件失败（%s）：%w", p, err)
		}
		return string(raw), nil
	}

	title, err := load("title.md")
	if err != nil {
		return SectionRules{}, err
	}
	bullets, err := load("bullets.md")
	if err != nil {
		return SectionRules{}, err
	}
	desc, err := load("description.md")
	if err != nil {
		return SectionRules{}, err
	}
	search, err := load("search_terms.md")
	if err != nil {
		return SectionRules{}, err
	}
	return SectionRules{
		Title:       title,
		Bullets:     bullets,
		Description: desc,
		SearchTerms: search,
	}, nil
}

func ensureRuleFiles(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("规则目录为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建规则目录失败（%s）：%w", dir, err)
	}

	files := []string{"title.md", "bullets.md", "description.md", "search_terms.md"}
	for _, name := range files {
		target := filepath.Join(dir, name)
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			continue
		}
		content, err := embeddedRuleFiles.ReadFile(filepath.Join("default_rules", name))
		if err != nil {
			return fmt.Errorf("读取内置规则失败（%s）：%w", name, err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return fmt.Errorf("写入默认规则失败（%s）：%w", target, err)
		}
	}
	return nil
}
