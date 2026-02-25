package app

import (
	"fmt"
	"strings"

	"syl-listing/internal/config"
)

func ensureDeepSeekAPIKey(paths *config.Paths, keyName string) (map[string]string, string, error) {
	keyName = strings.TrimSpace(keyName)
	if keyName == "" {
		keyName = "DEEPSEEK_API_KEY"
	}

	envMap, err := config.LoadEnvFile(paths.EnvPath)
	if err != nil {
		return nil, "", fmt.Errorf("尚未配置 API KEY\n执行：syl-listing set key <api_key>")
	}
	key := strings.TrimSpace(envMap[keyName])
	if key == "" {
		return nil, "", fmt.Errorf("尚未配置 API KEY\n执行：syl-listing set key <api_key>")
	}
	return envMap, key, nil
}
