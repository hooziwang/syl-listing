package config

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildRulesTarGz(t *testing.T) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	files := map[string]string{
		"title.yaml":        "x",
		"bullets.yaml":      "x",
		"description.yaml":  "x",
		"search_terms.yaml": "x",
	}
	for name, content := range files {
		h := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRulesCenterHelpers(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "rules.lock")
	l := rulesLock{ReleaseID: 1, TagName: "t", AssetName: "a"}
	if err := writeRulesLock(lockPath, l); err != nil {
		t.Fatal(err)
	}
	got, err := readRulesLock(lockPath)
	if err != nil || got.ReleaseID != 1 {
		t.Fatalf("read lock failed: %+v err=%v", got, err)
	}
	if fallbackTag("", "x") != "x" {
		t.Fatalf("fallbackTag mismatch")
	}
}

func TestApplyRulesBundle(t *testing.T) {
	d := t.TempDir()
	rulesDir := filepath.Join(d, "rules")
	raw := buildRulesTarGz(t)
	if err := applyRulesBundle(raw, rulesDir); err != nil {
		t.Fatalf("applyRulesBundle error: %v", err)
	}
	if !requiredRuleFilesExist(rulesDir) {
		t.Fatalf("required files missing")
	}
}

func TestApplyRulesBundleFailures(t *testing.T) {
	d := t.TempDir()
	rulesDir := filepath.Join(d, "rules")
	if err := applyRulesBundle([]byte("bad-gzip"), rulesDir); err == nil {
		t.Fatalf("expected invalid archive error")
	}

	// tar.gz with missing required files
	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "title.yaml", Mode: 0o644, Size: 1})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()
	if err := applyRulesBundle(buf.Bytes(), rulesDir); err == nil || !strings.Contains(err.Error(), "缺少") {
		t.Fatalf("expected missing files error, got %v", err)
	}
}

func TestFetchAndDownloadHelpers(t *testing.T) {
	assetBody := []byte("ok")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{ID: 1, TagName: "v1", Assets: []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}{{Name: "rules-bundle.tar.gz", URL: "http://example.com/a"}}})
		case "/repos/o/r/releases/tags/v1":
			_ = json.NewEncoder(w).Encode(githubRelease{ID: 2, TagName: "v1", Assets: []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}{{Name: "rules-bundle.tar.gz", URL: "http://example.com/a"}}})
		case "/a":
			_, _ = w.Write(assetBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	targetURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	baseTransport := ts.Client().Transport
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = targetURL.Scheme
		clone.URL.Host = targetURL.Host
		return baseTransport.RoundTrip(clone)
	})}

	rel, err := fetchGitHubRelease(context.Background(), client, "o", "r", "latest")
	if err != nil || rel.ID != 1 {
		t.Fatalf("fetch latest failed: rel=%+v err=%v", rel, err)
	}
	rel, err = fetchGitHubRelease(context.Background(), client, "o", "r", "v1")
	if err != nil || rel.ID != 2 {
		t.Fatalf("fetch tag failed: rel=%+v err=%v", rel, err)
	}
	raw, err := downloadBytes(context.Background(), client, "https://api.github.com/a")
	if err != nil || string(raw) != "ok" {
		t.Fatalf("downloadBytes failed: %q err=%v", string(raw), err)
	}

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer badSrv.Close()
	_, err = downloadBytes(context.Background(), badSrv.Client(), badSrv.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected download http error, got %v", err)
	}

	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer emptySrv.Close()
	_, err = downloadBytes(context.Background(), emptySrv.Client(), emptySrv.URL)
	if err == nil || !strings.Contains(err.Error(), "空响应") {
		t.Fatalf("expected empty response error, got %v", err)
	}
}

func TestSyncRulesFromCenterWarnings(t *testing.T) {
	d := t.TempDir()
	cfg := &Config{}
	cfg.applyDefaults()
	cfg.RulesCenter.Owner = "x"
	cfg.RulesCenter.Repo = "y"
	cfg.RulesCenter.TimeoutSec = 1
	cfg.RulesCenter.Strict = false
	paths := &Paths{ResolvedRulesDir: filepath.Join(d, "rules"), RulesLockPath: filepath.Join(d, "rules.lock")}
	if err := os.MkdirAll(paths.ResolvedRulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := SyncRulesFromCenter(cfg, paths)
	if err != nil {
		t.Fatalf("expected warning mode, got err: %v", err)
	}
	if strings.TrimSpace(res.Warning) == "" {
		t.Fatalf("expected warning")
	}
}

func TestSyncRulesFromCenterStrictOwnerRepo(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	cfg.RulesCenter.Owner = ""
	cfg.RulesCenter.Repo = ""
	cfg.RulesCenter.Strict = true
	_, err := SyncRulesFromCenter(cfg, &Paths{ResolvedRulesDir: t.TempDir(), RulesLockPath: filepath.Join(t.TempDir(), "rules.lock")})
	if err == nil {
		t.Fatalf("expected strict owner/repo error")
	}
}

func TestSyncRulesFromCenterSuccessAndNoUpdate(t *testing.T) {
	d := t.TempDir()
	bundle := buildRulesTarGz(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				ID:      11,
				TagName: "rules-v1",
				Assets: []struct {
					Name string `json:"name"`
					URL  string `json:"browser_download_url"`
				}{
					{Name: "rules-bundle.tar.gz", URL: "https://api.github.com/download/rules-bundle.tar.gz"},
				},
			})
		case "/download/rules-bundle.tar.gz":
			_, _ = w.Write(bundle)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	targetURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = targetURL.Scheme
		clone.URL.Host = targetURL.Host
		return ts.Client().Transport.RoundTrip(clone)
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	cfg := &Config{}
	cfg.applyDefaults()
	cfg.RulesCenter.Owner = "o"
	cfg.RulesCenter.Repo = "r"
	cfg.RulesCenter.Release = "latest"
	cfg.RulesCenter.Asset = "rules-bundle.tar.gz"
	cfg.RulesCenter.TimeoutSec = 3
	cfg.RulesCenter.Strict = true

	paths := &Paths{
		ResolvedRulesDir: filepath.Join(d, "rules"),
		RulesLockPath:    filepath.Join(d, "rules.lock"),
	}
	if err := os.MkdirAll(paths.ResolvedRulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := SyncRulesFromCenter(cfg, paths)
	if err != nil {
		t.Fatalf("sync should succeed, got err: %v", err)
	}
	if !res.Updated || !strings.Contains(res.Message, "更新成功") {
		t.Fatalf("unexpected sync result: %+v", res)
	}
	if !requiredRuleFilesExist(paths.ResolvedRulesDir) {
		t.Fatalf("rules should exist after sync")
	}
	lock, err := readRulesLock(paths.RulesLockPath)
	if err != nil || lock.ReleaseID != 11 {
		t.Fatalf("lock mismatch: %+v err=%v", lock, err)
	}

	res, err = SyncRulesFromCenter(cfg, paths)
	if err != nil {
		t.Fatalf("sync second call should succeed, got err: %v", err)
	}
	if res.Updated || !strings.Contains(res.Message, "最新版本") {
		t.Fatalf("expected no update path, got %+v", res)
	}
}

func TestSyncRulesFromCenterAssetMissingStrict(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/o/r/releases/latest" {
			_ = json.NewEncoder(w).Encode(githubRelease{ID: 1, TagName: "v1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	targetURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = targetURL.Scheme
		clone.URL.Host = targetURL.Host
		return ts.Client().Transport.RoundTrip(clone)
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	cfg := &Config{}
	cfg.applyDefaults()
	cfg.RulesCenter.Owner = "o"
	cfg.RulesCenter.Repo = "r"
	cfg.RulesCenter.Strict = true
	paths := &Paths{ResolvedRulesDir: filepath.Join(t.TempDir(), "rules"), RulesLockPath: filepath.Join(t.TempDir(), "rules.lock")}
	_, err = SyncRulesFromCenter(cfg, paths)
	if err == nil || !strings.Contains(err.Error(), "未找到资产") {
		t.Fatalf("expected strict missing asset error, got %v", err)
	}
}

func TestSyncRulesFromCenterStrictReleaseQueryError(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	cfg.RulesCenter.Owner = "x"
	cfg.RulesCenter.Repo = "y"
	cfg.RulesCenter.Strict = true
	cfg.RulesCenter.TimeoutSec = 1
	_, err := SyncRulesFromCenter(cfg, &Paths{ResolvedRulesDir: filepath.Join(t.TempDir(), "rules"), RulesLockPath: filepath.Join(t.TempDir(), "rules.lock")})
	if err == nil {
		t.Fatalf("expected strict release query error")
	}
}

func TestSyncRulesFromCenterOwnerRepoWarningNonStrict(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	cfg.RulesCenter.Owner = ""
	cfg.RulesCenter.Repo = ""
	cfg.RulesCenter.Strict = false
	res, err := SyncRulesFromCenter(cfg, &Paths{ResolvedRulesDir: t.TempDir(), RulesLockPath: filepath.Join(t.TempDir(), "rules.lock")})
	if err != nil {
		t.Fatalf("expected non-strict warning, got err=%v", err)
	}
	if !strings.Contains(res.Warning, "缺少 owner 或 repo") {
		t.Fatalf("expected owner/repo warning, got %+v", res)
	}
}

func TestSyncRulesFromCenterDownloadAndBundleWarnings(t *testing.T) {
	d := t.TempDir()
	t.Run("download warning", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/o/r/releases/latest":
				_ = json.NewEncoder(w).Encode(githubRelease{
					ID:      21,
					TagName: "rules-v21",
					Assets: []struct {
						Name string `json:"name"`
						URL  string `json:"browser_download_url"`
					}{
						{Name: "rules-bundle.tar.gz", URL: "https://api.github.com/download/rules-bundle.tar.gz"},
					},
				})
			case "/download/rules-bundle.tar.gz":
				http.Error(w, "down", http.StatusBadGateway)
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()
		targetURL, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		origTransport := http.DefaultTransport
		http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = targetURL.Scheme
			clone.URL.Host = targetURL.Host
			return ts.Client().Transport.RoundTrip(clone)
		})
		t.Cleanup(func() { http.DefaultTransport = origTransport })

		cfg := &Config{}
		cfg.applyDefaults()
		cfg.RulesCenter.Owner = "o"
		cfg.RulesCenter.Repo = "r"
		cfg.RulesCenter.Strict = false
		cfg.RulesCenter.TimeoutSec = 3

		res, err := SyncRulesFromCenter(cfg, &Paths{
			ResolvedRulesDir: filepath.Join(d, "rules-download"),
			RulesLockPath:    filepath.Join(d, "rules-download.lock"),
		})
		if err != nil {
			t.Fatalf("expected warning mode, got err=%v", err)
		}
		if !strings.Contains(res.Warning, "规则中心下载失败") {
			t.Fatalf("expected download warning, got %+v", res)
		}
	})

	t.Run("bundle warning", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/o/r/releases/latest":
				_ = json.NewEncoder(w).Encode(githubRelease{
					ID:      22,
					TagName: "rules-v22",
					Assets: []struct {
						Name string `json:"name"`
						URL  string `json:"browser_download_url"`
					}{
						{Name: "rules-bundle.tar.gz", URL: "https://api.github.com/download/rules-bundle.tar.gz"},
					},
				})
			case "/download/rules-bundle.tar.gz":
				_, _ = w.Write([]byte("bad-gzip"))
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()
		targetURL, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		origTransport := http.DefaultTransport
		http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = targetURL.Scheme
			clone.URL.Host = targetURL.Host
			return ts.Client().Transport.RoundTrip(clone)
		})
		t.Cleanup(func() { http.DefaultTransport = origTransport })

		cfg := &Config{}
		cfg.applyDefaults()
		cfg.RulesCenter.Owner = "o"
		cfg.RulesCenter.Repo = "r"
		cfg.RulesCenter.Strict = false
		cfg.RulesCenter.TimeoutSec = 3

		res, err := SyncRulesFromCenter(cfg, &Paths{
			ResolvedRulesDir: filepath.Join(d, "rules-bundle"),
			RulesLockPath:    filepath.Join(d, "rules-bundle.lock"),
		})
		if err != nil {
			t.Fatalf("expected warning mode, got err=%v", err)
		}
		if !strings.Contains(res.Warning, "规则包解压失败") {
			t.Fatalf("expected bundle warning, got %+v", res)
		}
	})
}

func TestRequiredRuleFilesExistFalse(t *testing.T) {
	d := t.TempDir()
	if requiredRuleFilesExist(d) {
		t.Fatalf("empty dir should not satisfy required files")
	}
}

func TestSyncRulesFromCenterNilArgs(t *testing.T) {
	res, err := SyncRulesFromCenter(nil, nil)
	if err != nil {
		t.Fatalf("expected nil args no error, got %v", err)
	}
	if res.Updated || res.Warning != "" || res.Message != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestFetchGitHubReleaseInvalidJSONAndEmptyID(t *testing.T) {
	badJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{bad-json"))
	}))
	defer badJSON.Close()
	badURL, err := url.Parse(badJSON.URL)
	if err != nil {
		t.Fatal(err)
	}
	badClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = badURL.Scheme
		clone.URL.Host = badURL.Host
		return badJSON.Client().Transport.RoundTrip(clone)
	})}
	_, err = fetchGitHubRelease(context.Background(), badClient, "o", "r", "latest")
	if err == nil || !strings.Contains(err.Error(), "解析 release 响应失败") {
		t.Fatalf("expected release json error, got %v", err)
	}

	emptyID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{ID: 0})
	}))
	defer emptyID.Close()
	emptyURL, err := url.Parse(emptyID.URL)
	if err != nil {
		t.Fatal(err)
	}
	emptyClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = emptyURL.Scheme
		clone.URL.Host = emptyURL.Host
		return emptyID.Client().Transport.RoundTrip(clone)
	})}
	_, err = fetchGitHubRelease(context.Background(), emptyClient, "o", "r", "latest")
	if err == nil || !strings.Contains(err.Error(), "release id 为空") {
		t.Fatalf("expected release id empty error, got %v", err)
	}
}

func TestReadWriteRulesLockErrorPaths(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "rules.lock")
	if err := os.WriteFile(lockPath, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRulesLock(lockPath); err == nil {
		t.Fatalf("expected readRulesLock json error")
	}

	parentAsFile := filepath.Join(d, "parent-file")
	if err := os.WriteFile(parentAsFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeRulesLock(filepath.Join(parentAsFile, "rules.lock"), rulesLock{ReleaseID: 1})
	if err == nil {
		t.Fatalf("expected writeRulesLock error")
	}
}

func TestSyncRulesFromCenterWriteLockFailureStrictAndNonStrict(t *testing.T) {
	d := t.TempDir()
	bundle := buildRulesTarGz(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				ID:      12,
				TagName: "rules-v2",
				Assets: []struct {
					Name string `json:"name"`
					URL  string `json:"browser_download_url"`
				}{
					{Name: "rules-bundle.tar.gz", URL: "https://api.github.com/download/rules-bundle.tar.gz"},
				},
			})
		case "/download/rules-bundle.tar.gz":
			_, _ = w.Write(bundle)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	targetURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = targetURL.Scheme
		clone.URL.Host = targetURL.Host
		return ts.Client().Transport.RoundTrip(clone)
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	lockParentFile := filepath.Join(d, "lock-parent")
	if err := os.WriteFile(lockParentFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseCfg := &Config{}
	baseCfg.applyDefaults()
	baseCfg.RulesCenter.Owner = "o"
	baseCfg.RulesCenter.Repo = "r"
	baseCfg.RulesCenter.Release = "latest"
	baseCfg.RulesCenter.Asset = "rules-bundle.tar.gz"
	baseCfg.RulesCenter.TimeoutSec = 3

	paths := &Paths{
		ResolvedRulesDir: filepath.Join(d, "rules"),
		RulesLockPath:    filepath.Join(lockParentFile, "rules.lock"),
	}
	if err := os.MkdirAll(paths.ResolvedRulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	nonStrict := *baseCfg
	nonStrict.RulesCenter.Strict = false
	res, err := SyncRulesFromCenter(&nonStrict, paths)
	if err != nil {
		t.Fatalf("non-strict should return warning, got err=%v", err)
	}
	if !strings.Contains(res.Warning, "写入规则锁失败") {
		t.Fatalf("expected write lock warning, got %+v", res)
	}

	strict := *baseCfg
	strict.RulesCenter.Strict = true
	_, err = SyncRulesFromCenter(&strict, paths)
	if err == nil || !strings.Contains(err.Error(), "写入规则锁失败") {
		t.Fatalf("expected strict write lock error, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
