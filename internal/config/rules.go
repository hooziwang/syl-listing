package config

import "fmt"

type RuleIntConstraint struct {
	Value     int    `yaml:"value"`
	Count     string `yaml:"count"`
	Hard      bool   `yaml:"hard"`
	ExplainZH string `yaml:"explain_zh"`
}

type RuleKeywordConstraint struct {
	Value     int    `yaml:"value"`
	Source    string `yaml:"source"`
	Match     string `yaml:"match"`
	Hard      bool   `yaml:"hard"`
	ExplainZH string `yaml:"explain_zh"`
}

type RuleOutputSpec struct {
	Format             string `yaml:"format"`
	Lines              int    `yaml:"lines"`
	Paragraphs         int    `yaml:"paragraphs"`
	ParagraphSeparator string `yaml:"paragraph_separator"`
}

type RuleConstraints struct {
	MaxChars                RuleIntConstraint     `yaml:"max_chars"`
	MinCharsPerLine         RuleIntConstraint     `yaml:"min_chars_per_line"`
	MaxCharsPerLine         RuleIntConstraint     `yaml:"max_chars_per_line"`
	MustContainTopNKeywords RuleKeywordConstraint `yaml:"must_contain_top_n_keywords"`
}

type SectionRule struct {
	Version     int             `yaml:"version"`
	Section     string          `yaml:"section"`
	Language    string          `yaml:"language"`
	Purpose     string          `yaml:"purpose"`
	Output      RuleOutputSpec  `yaml:"output"`
	Constraints RuleConstraints `yaml:"constraints"`
	Forbidden   []string        `yaml:"forbidden"`
	Instruction string          `yaml:"instruction"`
}

type SectionRuleFile struct {
	Path   string
	Raw    string
	Parsed SectionRule
}

type SectionRules struct {
	Title       SectionRuleFile
	Bullets     SectionRuleFile
	Description SectionRuleFile
	SearchTerms SectionRuleFile
}

func (s SectionRules) Get(step string) (SectionRuleFile, error) {
	switch step {
	case "title":
		return s.Title, nil
	case "bullets":
		return s.Bullets, nil
	case "description":
		return s.Description, nil
	case "search_terms":
		return s.SearchTerms, nil
	default:
		return SectionRuleFile{}, fmt.Errorf("未知分段：%s", step)
	}
}

func (s SectionRules) BulletCount() int {
	return s.Bullets.Parsed.Output.Lines
}

func (s SectionRules) DescriptionParagraphs() int {
	return s.Description.Parsed.Output.Paragraphs
}
