package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		v = strings.Trim(v, "\"'")
		if k != "" {
			out[k] = v
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("读取 .env 失败：%w", err)
	}
	return out, nil
}

func UpsertEnvVar(path, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("env key 为空")
	}
	value = strings.TrimSpace(value)
	lines := make([]string, 0, 8)
	if raw, err := os.ReadFile(path); err == nil {
		text := strings.ReplaceAll(string(raw), "\r\n", "\n")
		lines = strings.Split(text, "\n")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取 .env 失败：%w", err)
	}

	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.Index(trimmed, "=")
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(trimmed[:idx])
		if k != key {
			continue
		}
		lines[i] = fmt.Sprintf("%s=%s", key, value)
		found = true
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 .env 目录失败：%w", err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("写入 .env 失败：%w", err)
	}
	return nil
}
