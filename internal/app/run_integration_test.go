package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func writeTestRules(t *testing.T, rulesDir string) {
	t.Helper()
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"title.yaml": `version: 1
section: title
language: en
purpose: title
output:
  format: plain_text
  lines: 1
constraints:
  max_chars:
    value: 200
  must_contain_top_n_keywords:
    value: 1
forbidden: []
execution:
  generation:
    protocol: text
  repair:
    granularity: whole
  fallback:
    disable_thinking_on_length_error: true
instruction: |
  only output title
`,
		"bullets.yaml": `version: 1
section: bullets
language: en
purpose: bullets
output:
  format: json_object
  lines: 5
constraints:
  min_chars_per_line:
    value: 10
  max_chars_per_line:
    value: 120
forbidden: []
execution:
  generation:
    protocol: json_lines
  repair:
    granularity: item
    item_json_field: item
  fallback:
    disable_thinking_on_length_error: true
instruction: |
  only output bullets json
`,
		"description.yaml": `version: 1
section: description
language: en
purpose: desc
output:
  format: plain_text
  paragraphs: 2
constraints: {}
forbidden: []
execution:
  generation:
    protocol: text
  repair:
    granularity: whole
  fallback:
    disable_thinking_on_length_error: true
instruction: |
  only output description
`,
		"search_terms.yaml": `version: 1
section: search_terms
language: en
purpose: search
output:
  format: plain_text
  lines: 1
constraints:
  max_chars:
    value: 120
forbidden: []
execution:
  generation:
    protocol: text
  repair:
    granularity: whole
  fallback:
    disable_thinking_on_length_error: true
instruction: |
  only output search terms
`,
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunEndToEndWithMockDeepSeek(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	var apiCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			apiCalls++
			var req struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(req.Messages) == 0 {
				http.Error(w, "empty messages", http.StatusBadRequest)
				return
			}
			system := req.Messages[0].Content
			user := req.Messages[len(req.Messages)-1].Content
			if strings.Contains(system, "你是专业翻译") {
				fmt.Fprintf(w, `{"choices":[{"message":{"content":"中:%s"}}]}`, strings.ReplaceAll(user, `"`, `'`))
				return
			}
			step := ""
			re := regexp.MustCompile(`【当前任务】生成：([^\n]+)`)
			m := re.FindStringSubmatch(user)
			if len(m) == 2 {
				step = strings.TrimSpace(m[1])
			}
			var out string
			switch step {
			case "title":
				out = "alpha decorative title"
			case "bullets":
				out = `{"bullets":["bullet line 1 enough","bullet line 2 enough","bullet line 3 enough","bullet line 4 enough","bullet line 5 enough"]}`
			case "description":
				out = "paragraph one for product.\n\nparagraph two for usage."
			case "search_terms":
				out = "alpha beta gamma"
			default:
				out = "fallback text"
			}
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, out)
		case "/user/balance":
			fmt.Fprint(w, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"12.34"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	origDefault := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = origDefault })
	targetURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	baseTransport := server.Client().Transport
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = targetURL.Scheme
			clone.URL.Host = targetURL.Host
			return baseTransport.RoundTrip(clone)
		}),
	}

	cfgRoot := filepath.Join(home, ".syl-listing")
	if err := os.MkdirAll(cfgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgRoot, "config.yaml")
	cfgContent := fmt.Sprintf(`provider: deepseek
api_key_env: DEEPSEEK_API_KEY
rules_center:
  owner: x
  repo: y
  release: latest
  asset: rules-bundle.tar.gz
  timeout_sec: 1
  strict: false
char_tolerance: 20
concurrency: 0
max_retries: 1
request_timeout_sec: 30
output:
  dir: .
  num: 1
providers:
  deepseek:
    base_url: %s
    api_mode: chat
    model: deepseek-chat
    model_reasoning_effort: ""
    thinking_fallback:
      enabled: false
      attempt: 3
      model: deepseek-reasoner
`, server.URL)
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgRoot, ".env"), []byte("DEEPSEEK_API_KEY=test-key\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && cacheDir == "" {
		t.Fatal("empty cache dir on windows")
	}
	rulesDir := filepath.Join(cacheDir, "syl-listing", "rules")
	writeTestRules(t, rulesDir)

	workDir := t.TempDir()
	reqPath := filepath.Join(workDir, "req.md")
	req := strings.Join([]string{
		"===Listing Requirements===",
		"品牌名: BrandX",
		"分类: Home > Decor",
		"# 关键词库",
		"- alpha",
		"- beta",
		"- gamma",
	}, "\n")
	if err := os.WriteFile(reqPath, []byte(req), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	res, err := Run(Options{
		Inputs:     []string{reqPath},
		ConfigPath: cfgPath,
		CWD:        workDir,
		Stdout:     &out,
		Stderr:     &out,
	})
	if err != nil {
		t.Fatalf("Run error: %v\nlogs:\n%s", err, out.String())
	}
	if res.Succeeded != 1 || res.Failed != 0 {
		t.Fatalf("unexpected result: %+v\nlogs:\n%s", res, out.String())
	}
	if res.Balance != "12.34 元" {
		t.Fatalf("unexpected balance: %s", res.Balance)
	}
	if apiCalls == 0 {
		t.Fatalf("expected api calls")
	}

	enFiles, _ := filepath.Glob(filepath.Join(workDir, "listing_*_en.md"))
	cnFiles, _ := filepath.Glob(filepath.Join(workDir, "listing_*_cn.md"))
	if len(enFiles) != 1 || len(cnFiles) != 1 {
		t.Fatalf("expected 1 en + 1 cn files, got en=%d cn=%d", len(enFiles), len(cnFiles))
	}
	cnRaw, _ := os.ReadFile(cnFiles[0])
	if !strings.Contains(string(cnRaw), "中:") {
		t.Fatalf("expected translated content in cn file: %s", string(cnRaw))
	}
}
