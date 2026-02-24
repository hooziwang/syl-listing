package output

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func EnsureDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("输出目录为空")
	}
	return os.MkdirAll(dir, 0o755)
}

func NextPair(dir string, randomLen int, randSrc io.Reader) (id string, enPath string, cnPath string, err error) {
	if randomLen <= 0 {
		randomLen = 8
	}
	if randSrc == nil {
		randSrc = rand.Reader
	}
	for i := 0; i < 1000; i++ {
		id, err = randomID(randomLen, randSrc)
		if err != nil {
			return "", "", "", err
		}
		enPath = filepath.Join(dir, fmt.Sprintf("listing_%s_en.md", id))
		cnPath = filepath.Join(dir, fmt.Sprintf("listing_%s_cn.md", id))
		if !exists(enPath) && !exists(cnPath) {
			return id, enPath, cnPath, nil
		}
	}
	return "", "", "", fmt.Errorf("尝试多次仍无法生成不冲突文件名")
}

func NextEN(dir string, randomLen int, randSrc io.Reader) (id string, enPath string, err error) {
	if randomLen <= 0 {
		randomLen = 8
	}
	if randSrc == nil {
		randSrc = rand.Reader
	}
	for i := 0; i < 1000; i++ {
		id, err = randomID(randomLen, randSrc)
		if err != nil {
			return "", "", err
		}
		enPath = filepath.Join(dir, fmt.Sprintf("listing_%s_en.md", id))
		if !exists(enPath) {
			return id, enPath, nil
		}
	}
	return "", "", fmt.Errorf("尝试多次仍无法生成不冲突文件名")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func randomID(n int, randSrc io.Reader) (string, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(randSrc, buf); err != nil {
		return "", fmt.Errorf("读取随机数失败：%w", err)
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
