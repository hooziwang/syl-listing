package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"syl-listing/internal/config"
)

func withBalanceEndpoint(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	targetURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	orig := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = targetURL.Scheme
			clone.URL.Host = targetURL.Host
			return ts.Client().Transport.RoundTrip(clone)
		}),
	}
	t.Cleanup(func() { http.DefaultClient = orig })
}

func TestEnsureDeepSeekAPIKey_ExistingKey(t *testing.T) {
	d := t.TempDir()
	envPath := filepath.Join(d, ".env")
	if err := config.UpsertEnvVar(envPath, "DEEPSEEK_API_KEY", "existing"); err != nil {
		t.Fatal(err)
	}
	envMap, key, err := ensureDeepSeekAPIKey(&config.Paths{EnvPath: envPath}, "DEEPSEEK_API_KEY", 0, &bytes.Buffer{}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("expected existing key success, got %v", err)
	}
	if key != "existing" || envMap["DEEPSEEK_API_KEY"] != "existing" {
		t.Fatalf("unexpected key/map: key=%q map=%v", key, envMap)
	}
}

func TestEnsureDeepSeekAPIKey_PromptThenSave(t *testing.T) {
	withBalanceEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), "Bearer valid-key") {
			fmt.Fprint(w, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"1.23"}]}`)
			return
		}
		http.Error(w, "bad", http.StatusUnauthorized)
	})

	d := t.TempDir()
	envPath := filepath.Join(d, ".env")
	var out bytes.Buffer
	envMap, key, err := ensureDeepSeekAPIKey(
		&config.Paths{EnvPath: envPath},
		"DEEPSEEK_API_KEY",
		0,
		&out,
		strings.NewReader("invalid\nvalid-key\n"),
	)
	if err != nil {
		t.Fatalf("expected prompt flow success, got %v", err)
	}
	if key != "valid-key" || envMap["DEEPSEEK_API_KEY"] != "valid-key" {
		t.Fatalf("unexpected key/map: key=%q map=%v", key, envMap)
	}
	loaded, err := config.LoadEnvFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded["DEEPSEEK_API_KEY"] != "valid-key" {
		t.Fatalf("key not persisted, env=%v", loaded)
	}
	text := out.String()
	if !strings.Contains(text, "无效 Key") || strings.Contains(text, "已保存到") {
		t.Fatalf("unexpected prompt output: %s", text)
	}
}

func TestEnsureDeepSeekAPIKey_EOFError(t *testing.T) {
	d := t.TempDir()
	envPath := filepath.Join(d, ".env")
	_, _, err := ensureDeepSeekAPIKey(&config.Paths{EnvPath: envPath}, "DEEPSEEK_API_KEY", 0, &bytes.Buffer{}, strings.NewReader("\n"))
	if err == nil || !strings.Contains(err.Error(), "未读取到有效") {
		t.Fatalf("expected eof error, got %v", err)
	}
}
