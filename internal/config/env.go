package config

import (
	"bufio"
	"fmt"
	"os"
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
