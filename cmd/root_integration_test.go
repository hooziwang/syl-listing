package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func writeCmdRules(t *testing.T, rulesDir string) {
	t.Helper()
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"title.yaml": `version: 1
section: title
language: en
purpose: t
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
instruction: x
`,
		"bullets.yaml": `version: 1
section: bullets
language: en
purpose: b
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
instruction: x
`,
		"description.yaml": `version: 1
section: description
language: en
purpose: d
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
instruction: x
`,
		"search_terms.yaml": `version: 1
section: search_terms
language: en
purpose: s
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
instruction: x
`,
	}
	for n, raw := range files {
		if err := os.WriteFile(filepath.Join(rulesDir, n), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRootCmdRunGenSuccessPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			var req struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if len(req.Messages) == 0 {
				http.Error(w, "empty", http.StatusBadRequest)
				return
			}
			sys := req.Messages[0].Content
			user := req.Messages[len(req.Messages)-1].Content
			if strings.Contains(sys, "你是专业翻译") {
				fmt.Fprintf(w, `{"choices":[{"message":{"content":"中:%s"}}]}`, strings.ReplaceAll(user, `"`, `'`))
				return
			}
			re := regexp.MustCompile(`【当前任务】生成：([^\n]+)`)
			m := re.FindStringSubmatch(user)
			step := ""
			if len(m) == 2 {
				step = strings.TrimSpace(m[1])
			}
			out := "fallback"
			switch step {
			case "title":
				out = "alpha generated title"
			case "bullets":
				out = `{"bullets":["bullet line 1 enough","bullet line 2 enough","bullet line 3 enough","bullet line 4 enough","bullet line 5 enough"]}`
			case "description":
				out = "desc one.\n\ndesc two."
			case "search_terms":
				out = "alpha beta"
			}
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, out)
		case "/user/balance":
			fmt.Fprint(w, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"8.88"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	origDefault := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return server.Client().Transport.RoundTrip(clone)
	})}
	t.Cleanup(func() { http.DefaultClient = origDefault })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	cfgRoot := filepath.Join(home, ".syl-listing")
	if err := os.MkdirAll(cfgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgRoot, "config.yaml")
	cfg := fmt.Sprintf(`provider: deepseek
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
`, server.URL)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgRoot, ".env"), []byte("DEEPSEEK_API_KEY=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	writeCmdRules(t, filepath.Join(cacheDir, "syl-listing", "rules"))

	work := t.TempDir()
	reqPath := filepath.Join(work, "req.md")
	req := strings.Join([]string{
		"===Listing Requirements===",
		"品牌名: BrandX",
		"分类: Cat",
		"# 关键词库",
		"- alpha",
		"- beta",
		"- gamma",
	}, "\n")
	if err := os.WriteFile(reqPath, []byte(req), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := os.Create(filepath.Join(work, "stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errf, err := os.Create(filepath.Join(work, "stderr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer errf.Close()

	root := NewRootCmd(out, errf)
	root.SetArgs([]string{"gen", reqPath, "--config", cfgPath, "-o", work})
	if err := root.Execute(); err != nil {
		t.Fatalf("root execute failed: %v", err)
	}
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "任务完成：成功 1，失败 0") {
		t.Fatalf("unexpected stdout: %s", string(raw))
	}
}
