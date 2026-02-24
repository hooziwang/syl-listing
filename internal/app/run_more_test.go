package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupRunFixture(t *testing.T, envContent string) (cfgPath string, workDir string) {
	t.Helper()
	home := t.TempDir()
	workDir = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/releases/latest"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       1,
				"tag_name": "v-test",
				"assets":   []any{},
			})
		case r.URL.Path == "/user/balance":
			fmt.Fprint(w, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"9.99"}]}`)
		case r.URL.Path == "/chat/completions":
			fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)

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

	cfgRoot := filepath.Join(home, ".syl-listing")
	if err := os.MkdirAll(cfgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath = filepath.Join(cfgRoot, "config.yaml")
	cfg := fmt.Sprintf(`provider: deepseek
api_key_env: DEEPSEEK_API_KEY
rules_center:
  owner: o
  repo: r
  release: latest
  asset: rules-bundle.tar.gz
  timeout_sec: 1
  strict: false
char_tolerance: 20
concurrency: 0
max_retries: 0
request_timeout_sec: 20
output:
  dir: .
  num: 1
providers:
  deepseek:
    base_url: %s
    api_mode: chat
    model: deepseek-chat
    model_reasoning_effort: ""
`, ts.URL)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgRoot, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	writeTestRules(t, filepath.Join(cacheDir, "syl-listing", "rules"))
	return cfgPath, workDir
}

func TestRunDiscoveryErrorWithNoInputs(t *testing.T) {
	cfgPath, workDir := setupRunFixture(t, "DEEPSEEK_API_KEY=test\n")
	_, err := Run(Options{
		Inputs:     nil,
		ConfigPath: cfgPath,
		CWD:        workDir,
		Stdout:     ioDiscard{},
		Stderr:     ioDiscard{},
	})
	if err == nil || !strings.Contains(err.Error(), "未提供输入路径") {
		t.Fatalf("expected discovery input error, got %v", err)
	}
}

func TestRunOnlyValidationFailuresReturnsResult(t *testing.T) {
	cfgPath, workDir := setupRunFixture(t, "DEEPSEEK_API_KEY=test\n")
	reqPath := filepath.Join(workDir, "bad.md")
	raw := strings.Join([]string{
		"===Listing Requirements===",
		"分类: Cat",
		"# 关键词库",
		"- alpha",
		"- beta",
	}, "\n")
	if err := os.WriteFile(reqPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{
		Inputs:     []string{reqPath},
		ConfigPath: cfgPath,
		CWD:        workDir,
		Stdout:     ioDiscard{},
		Stderr:     ioDiscard{},
	})
	if err != nil {
		t.Fatalf("expected nil err with failed validations only, got %v", err)
	}
	if res.Succeeded != 0 || res.Failed != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunOutputDirCreationError(t *testing.T) {
	cfgPath, workDir := setupRunFixture(t, "DEEPSEEK_API_KEY=test\n")
	reqPath := filepath.Join(workDir, "ok.md")
	raw := strings.Join([]string{
		"===Listing Requirements===",
		"品牌名: BrandX",
		"分类: Cat",
		"# 关键词库",
		"- alpha",
		"- beta",
	}, "\n")
	if err := os.WriteFile(reqPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	outAsFile := filepath.Join(workDir, "out-file")
	if err := os.WriteFile(outAsFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(Options{
		Inputs:     []string{reqPath},
		ConfigPath: cfgPath,
		OutputDir:  outAsFile,
		CWD:        workDir,
		Stdout:     ioDiscard{},
		Stderr:     ioDiscard{},
	})
	if err == nil || !strings.Contains(err.Error(), "创建输出目录失败") {
		t.Fatalf("expected output dir error, got %v", err)
	}
}

func TestRunMissingAPIKeyValue(t *testing.T) {
	cfgPath, workDir := setupRunFixture(t, "DEEPSEEK_API_KEY=\n")
	reqPath := filepath.Join(workDir, "ok.md")
	raw := strings.Join([]string{
		"===Listing Requirements===",
		"品牌名: BrandX",
		"分类: Cat",
		"# 关键词库",
		"- alpha",
		"- beta",
	}, "\n")
	if err := os.WriteFile(reqPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(Options{
		Inputs:     []string{reqPath},
		ConfigPath: cfgPath,
		CWD:        workDir,
		Stdout:     ioDiscard{},
		Stderr:     ioDiscard{},
		Stdin:      strings.NewReader("\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "未读取到有效的 DEEPSEEK_API_KEY 输入") {
		t.Fatalf("expected prompt eof error, got %v", err)
	}
}

func TestRunWithEmptyCWDUsesGetwd(t *testing.T) {
	cfgPath, workDir := setupRunFixture(t, "DEEPSEEK_API_KEY=test\n")
	reqPath := filepath.Join(workDir, "ok.md")
	raw := strings.Join([]string{
		"===Listing Requirements===",
		"品牌名: BrandX",
		"分类: Cat",
		"# 关键词库",
		"- alpha",
		"- beta",
		"- gamma",
	}, "\n")
	if err := os.WriteFile(reqPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{
		Inputs:     []string{reqPath},
		ConfigPath: cfgPath,
		OutputDir:  workDir,
		CWD:        "",
		Stdout:     ioDiscard{},
		Stderr:     ioDiscard{},
	})
	if err != nil {
		t.Fatalf("expected success with empty cwd, got %v", err)
	}
	if res.Succeeded+res.Failed == 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (n int, err error) { return len(p), nil }
