package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var embeddedDefaultConfig []byte

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
	paths.ResolvedRulesDir = paths.RulesDir
	if err := ensureRuleDir(paths.ResolvedRulesDir); err != nil {
		return nil, nil, err
	}
	return cfg, paths, nil
}

func resolvePaths(configArg string) (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("读取用户目录失败：%w", err)
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = filepath.Join(home, ".cache")
	}

	root := filepath.Join(home, ".syl-listing")
	rulesRoot := filepath.Join(cacheRoot, "syl-listing")
	configPath := filepath.Join(root, "config.yaml")
	if strings.TrimSpace(configArg) != "" {
		configPath = expandPath(configArg, home, "")
	}

	return &Paths{
		HomeDir:       home,
		RootDir:       root,
		ConfigPath:    configPath,
		RulesDir:      filepath.Join(rulesRoot, "rules"),
		RulesLockPath: filepath.Join(rulesRoot, "rules.lock"),
		EnvPath:       filepath.Join(root, ".env"),
		EnvExample:    filepath.Join(root, ".env.example"),
	}, nil
}

func ensureBootstrap(paths *Paths) error {
	if err := os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败：%w", err)
	}
	if err := ensureFile(paths.ConfigPath, embeddedDefaultConfig, 0o644); err != nil {
		return err
	}
	if err := ensureRuleDir(paths.RulesDir); err != nil {
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

func ReadSectionRules(dir string) (SectionRules, error) {
	load := func(name string) (SectionRuleFile, error) {
		p := filepath.Join(dir, name)
		raw, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				return SectionRuleFile{}, fmt.Errorf("缺少规则文件（%s）。规则由规则中心自动同步到本机缓存", p)
			}
			return SectionRuleFile{}, fmt.Errorf("读取规则文件失败（%s）：%w", p, err)
		}
		rule := SectionRule{}
		if err := yaml.Unmarshal(raw, &rule); err != nil {
			return SectionRuleFile{}, fmt.Errorf("规则文件格式错误（%s）：%w", p, err)
		}
		return SectionRuleFile{Path: p, Raw: string(raw), Parsed: rule}, validateSectionRule(rule, name, p)
	}

	title, err := load("title.yaml")
	if err != nil {
		return SectionRules{}, err
	}
	bullets, err := load("bullets.yaml")
	if err != nil {
		return SectionRules{}, err
	}
	desc, err := load("description.yaml")
	if err != nil {
		return SectionRules{}, err
	}
	search, err := load("search_terms.yaml")
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

func ensureRuleDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("规则目录为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建规则目录失败（%s）：%w", dir, err)
	}
	return nil
}

func validateSectionRule(rule SectionRule, filename, path string) error {
	if strings.TrimSpace(rule.Instruction) == "" {
		return fmt.Errorf("规则文件缺少 instruction（%s）", path)
	}
	expectedSection := strings.TrimSuffix(filename, ".yaml")
	if expectedSection == "search_terms" {
		expectedSection = "search_terms"
	}
	if strings.TrimSpace(rule.Section) != expectedSection {
		return fmt.Errorf("规则文件 section 不匹配（%s）：期望 %s，实际 %s", path, expectedSection, strings.TrimSpace(rule.Section))
	}
	switch expectedSection {
	case "title":
		if rule.Output.Lines != 1 {
			return fmt.Errorf("title 规则 output.lines 必须为 1（%s）", path)
		}
		if rule.Constraints.MaxChars.Value <= 0 {
			return fmt.Errorf("title 规则 max_chars.value 必须 > 0（%s）", path)
		}
	case "bullets":
		if rule.Output.Lines <= 0 {
			return fmt.Errorf("bullets 规则 output.lines 必须 > 0（%s）", path)
		}
		if rule.Constraints.MinCharsPerLine.Value <= 0 || rule.Constraints.MaxCharsPerLine.Value <= 0 {
			return fmt.Errorf("bullets 规则每行长度约束必须 > 0（%s）", path)
		}
		if rule.Constraints.MinCharsPerLine.Value > rule.Constraints.MaxCharsPerLine.Value {
			return fmt.Errorf("bullets 规则最小长度不能大于最大长度（%s）", path)
		}
	case "description":
		if rule.Output.Paragraphs <= 0 {
			return fmt.Errorf("description 规则 output.paragraphs 必须 > 0（%s）", path)
		}
	case "search_terms":
		if rule.Output.Lines != 1 {
			return fmt.Errorf("search_terms 规则 output.lines 必须为 1（%s）", path)
		}
		if rule.Constraints.MaxChars.Value <= 0 {
			return fmt.Errorf("search_terms 规则 max_chars.value 必须 > 0（%s）", path)
		}
	}
	return nil
}
