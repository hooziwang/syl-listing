package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"syl-listing/internal/listing"
)

type Result struct {
	Files    []string
	Warnings []string
}

func Discover(inputs []string) (Result, error) {
	if len(inputs) == 0 {
		return Result{}, fmt.Errorf("未提供输入路径")
	}
	set := map[string]struct{}{}
	warnings := []string{}

	for _, in := range inputs {
		if strings.TrimSpace(in) == "" {
			continue
		}
		st, err := os.Stat(in)
		if err != nil {
			return Result{}, fmt.Errorf("输入路径无效（%s）：%w", in, err)
		}
		if st.IsDir() {
			dirs, warns, err := scanDir(in)
			if err != nil {
				return Result{}, err
			}
			for _, w := range warns {
				warnings = append(warnings, w)
			}
			for _, p := range dirs {
				set[p] = struct{}{}
			}
			continue
		}

		raw, err := os.ReadFile(in)
		if err != nil {
			return Result{}, fmt.Errorf("读取文件失败（%s）：%w", in, err)
		}
		if !listing.IsListingRequirements(string(raw)) {
			return Result{}, fmt.Errorf("文件不是 listing 需求格式（缺少首行标志）：%s", in)
		}
		set[in] = struct{}{}
	}

	files := make([]string, 0, len(set))
	for p := range set {
		files = append(files, p)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return Result{}, fmt.Errorf("未找到任何可用 listing 需求文件")
	}
	return Result{Files: files, Warnings: warnings}, nil
}

func scanDir(root string) ([]string, []string, error) {
	out := []string{}
	warnings := []string{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			warnings = append(warnings, fmt.Sprintf("读取失败已跳过：%s", path))
			return nil
		}
		if listing.IsListingRequirements(string(raw)) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("扫描目录失败（%s）：%w", root, err)
	}
	return out, warnings, nil
}
