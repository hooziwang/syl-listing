package config

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RulesSyncResult struct {
	Updated bool
	Message string
	Warning string
}

type rulesLock struct {
	ReleaseID int64  `json:"release_id"`
	TagName   string `json:"tag_name"`
	AssetName string `json:"asset_name"`
	SyncedAt  string `json:"synced_at"`
}

type githubRelease struct {
	ID      int64  `json:"id"`
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func SyncRulesFromCenter(cfg *Config, paths *Paths) (RulesSyncResult, error) {
	out := RulesSyncResult{}
	if cfg == nil || paths == nil {
		return out, nil
	}
	center := cfg.RulesCenter
	owner := strings.TrimSpace(center.Owner)
	repo := strings.TrimSpace(center.Repo)
	if owner == "" || repo == "" {
		msg := "rules_center 缺少 owner 或 repo"
		if center.Strict {
			return out, fmt.Errorf(msg)
		}
		out.Warning = msg
		return out, nil
	}

	releaseRef := strings.TrimSpace(center.Release)
	if releaseRef == "" {
		releaseRef = "latest"
	}
	assetName := strings.TrimSpace(center.Asset)
	if assetName == "" {
		assetName = "rules-bundle.tar.gz"
	}

	timeoutSec := center.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}

	release, err := fetchGitHubRelease(ctx, client, owner, repo, releaseRef)
	if err != nil {
		msg := fmt.Sprintf("规则中心查询失败：%v", err)
		if center.Strict {
			return out, fmt.Errorf(msg)
		}
		out.Warning = msg
		return out, nil
	}

	assetURL := ""
	for _, a := range release.Assets {
		if strings.EqualFold(strings.TrimSpace(a.Name), assetName) {
			assetURL = strings.TrimSpace(a.URL)
			break
		}
	}
	if assetURL == "" {
		msg := fmt.Sprintf("规则中心未找到资产 %s（release=%s）", assetName, fallbackTag(release.TagName, releaseRef))
		if center.Strict {
			return out, fmt.Errorf(msg)
		}
		out.Warning = msg
		return out, nil
	}

	lock, _ := readRulesLock(paths.RulesLockPath)
	if lock.ReleaseID == release.ID && lock.AssetName == assetName && requiredRuleFilesExist(paths.ResolvedRulesDir) {
		out.Message = fmt.Sprintf("规则已是最新版本（%s）", fallbackTag(release.TagName, releaseRef))
		return out, nil
	}

	raw, err := downloadBytes(ctx, client, assetURL)
	if err != nil {
		msg := fmt.Sprintf("规则中心下载失败：%v", err)
		if center.Strict {
			return out, fmt.Errorf(msg)
		}
		out.Warning = msg
		return out, nil
	}

	if err := applyRulesBundle(raw, paths.ResolvedRulesDir); err != nil {
		msg := fmt.Sprintf("规则包解压失败：%v", err)
		if center.Strict {
			return out, fmt.Errorf(msg)
		}
		out.Warning = msg
		return out, nil
	}

	newLock := rulesLock{
		ReleaseID: release.ID,
		TagName:   release.TagName,
		AssetName: assetName,
		SyncedAt:  time.Now().Format(time.RFC3339),
	}
	if err := writeRulesLock(paths.RulesLockPath, newLock); err != nil {
		msg := fmt.Sprintf("写入规则锁失败：%v", err)
		if center.Strict {
			return out, fmt.Errorf(msg)
		}
		out.Warning = msg
		return out, nil
	}
	out.Updated = true
	out.Message = fmt.Sprintf("规则中心更新成功（%s）", fallbackTag(release.TagName, releaseRef))
	return out, nil
}

func fetchGitHubRelease(ctx context.Context, client *http.Client, owner, repo, releaseRef string) (githubRelease, error) {
	var url string
	if strings.EqualFold(releaseRef, "latest") {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, releaseRef)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "syl-listing")
	resp, err := client.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return githubRelease{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out githubRelease
	if err := json.Unmarshal(body, &out); err != nil {
		return githubRelease{}, fmt.Errorf("解析 release 响应失败：%w", err)
	}
	if out.ID == 0 {
		return githubRelease{}, fmt.Errorf("release id 为空")
	}
	return out, nil
}

func downloadBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "syl-listing")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("空响应")
	}
	return raw, nil
}

func applyRulesBundle(raw []byte, rulesDir string) error {
	tmpDir := rulesDir + ".tmp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractRuleFiles(raw, tmpDir); err != nil {
		return err
	}
	for _, name := range requiredRuleFiles() {
		p := filepath.Join(tmpDir, name)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("规则包缺少 %s", name)
		}
	}
	if err := os.RemoveAll(rulesDir); err != nil {
		return err
	}
	if err := os.Rename(tmpDir, rulesDir); err != nil {
		return err
	}
	return nil
}

func extractRuleFiles(raw []byte, outDir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	required := map[string]struct{}{}
	for _, n := range requiredRuleFiles() {
		required[n] = struct{}{}
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(strings.TrimSpace(hdr.Name))
		if _, ok := required[base]; !ok {
			continue
		}
		target := filepath.Join(outDir, base)
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func requiredRuleFiles() []string {
	return []string{"title.yaml", "bullets.yaml", "description.yaml", "search_terms.yaml"}
}

func requiredRuleFilesExist(dir string) bool {
	for _, name := range requiredRuleFiles() {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

func readRulesLock(path string) (rulesLock, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return rulesLock{}, err
	}
	out := rulesLock{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return rulesLock{}, err
	}
	return out, nil
}

func writeRulesLock(path string, lock rulesLock) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func fallbackTag(tag, fallback string) string {
	if strings.TrimSpace(tag) != "" {
		return strings.TrimSpace(tag)
	}
	return strings.TrimSpace(fallback)
}
