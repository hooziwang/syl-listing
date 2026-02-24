package listing

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const Marker = "===Listing Requirements==="

type Requirement struct {
	SourcePath      string
	Raw             string
	BodyAfterMarker string
	Brand           string
	Category        string
	Keywords        []string
	Warnings        []string
}

func ParseFile(path string) (Requirement, error) {
	rawBytes, err := os.ReadFile(path)
	if err != nil {
		return Requirement{}, fmt.Errorf("读取文件失败（%s）：%w", path, err)
	}
	raw := string(rawBytes)
	body, ok := BodyAfterMarker(raw)
	if !ok {
		return Requirement{}, fmt.Errorf("文件不是 listing 需求格式（缺少首行标志 %s）：%s", Marker, path)
	}

	req := Requirement{
		SourcePath:      path,
		Raw:             raw,
		BodyAfterMarker: body,
		Brand:           parseBrand(body),
		Category:        parseCategory(body),
		Keywords:        parseKeywords(body),
	}
	if len(req.Keywords) < 15 || len(req.Keywords) > 20 {
		req.Warnings = append(req.Warnings, fmt.Sprintf("关键词数量是 %d，不在 15-20 范围，继续生成", len(req.Keywords)))
	}
	return req, nil
}

func IsListingRequirements(raw string) bool {
	_, ok := BodyAfterMarker(raw)
	return ok
}

func BodyAfterMarker(raw string) (string, bool) {
	raw = strings.TrimPrefix(raw, "\ufeff")
	lines := strings.Split(raw, "\n")
	idx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.TrimSpace(line) == Marker {
			idx = i
		}
		break
	}
	if idx < 0 {
		return "", false
	}
	if idx+1 >= len(lines) {
		return "", true
	}
	return strings.Join(lines[idx+1:], "\n"), true
}

func parseBrand(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "品牌名:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "品牌名:"))
		}
	}
	return ""
}

func parseCategory(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "分类:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "分类:"))
		}
		if strings.HasPrefix(trimmed, "# 分类") {
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" {
					continue
				}
				if strings.HasPrefix(next, "#") {
					return ""
				}
				return next
			}
		}
	}
	return ""
}

var keywordPrefixRe = regexp.MustCompile(`^([0-9]{1,2}[\.)]|[-*•])\s*`)

func parseKeywords(body string) []string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# 关键词库") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}

	out := make([]string, 0, 20)
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			break
		}
		kw := keywordPrefixRe.ReplaceAllString(line, "")
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		out = append(out, kw)
	}
	return out
}
