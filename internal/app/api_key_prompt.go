package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"syl-listing/internal/config"
)

func ensureDeepSeekAPIKey(paths *config.Paths, keyName string, maxRetries int, stdout io.Writer, stdin io.Reader) (map[string]string, string, error) {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stdin == nil {
		stdin = os.Stdin
	}
	keyName = strings.TrimSpace(keyName)
	if keyName == "" {
		keyName = "DEEPSEEK_API_KEY"
	}

	envMap := map[string]string{}
	if _, err := os.Stat(paths.EnvPath); err == nil {
		loaded, loadErr := config.LoadEnvFile(paths.EnvPath)
		if loadErr != nil {
			return nil, "", loadErr
		}
		envMap = loaded
	} else if err != nil && !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("检查 .env 失败（%s）：%w", paths.EnvPath, err)
	}

	if existing := strings.TrimSpace(envMap[keyName]); existing != "" {
		return envMap, existing, nil
	}

	reader := bufio.NewReader(stdin)
	for {
		fmt.Fprint(stdout, "请输入 API Key: ")
		line, readErr := reader.ReadString('\n')
		key := strings.TrimSpace(line)
		if key == "" {
			if readErr == io.EOF {
				return nil, "", fmt.Errorf("未读取到有效的 %s 输入", keyName)
			}
			fmt.Fprintln(stdout, "输入为空，请重新输入。")
			continue
		}

		if _, verifyErr := fetchDeepSeekBalanceWithRetry(key, maxRetries); verifyErr != nil {
			fmt.Fprintln(stdout, "无效 Key")
			if readErr == io.EOF {
				return nil, "", fmt.Errorf("%s 无效且无更多输入", keyName)
			}
			continue
		}

		if err := config.UpsertEnvVar(paths.EnvPath, keyName, key); err != nil {
			return nil, "", err
		}
		envMap[keyName] = key
		return envMap, key, nil
	}
}
