package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestResolveDeepSeekBalanceKey(t *testing.T) {
	if got := resolveDeepSeekBalanceKey(map[string]string{"DEEPSEEK_API_KEY": "x"}, ""); got != "x" {
		t.Fatalf("unexpected: %s", got)
	}
	if got := resolveDeepSeekBalanceKey(nil, "y"); got != "y" {
		t.Fatalf("unexpected: %s", got)
	}
	if got := resolveDeepSeekBalanceKey(nil, " "); got != "" {
		t.Fatalf("unexpected: %s", got)
	}
}

func TestBalanceFormattingHelpers(t *testing.T) {
	items := []deepSeekBalanceInfo{{Currency: "cny", TotalBalance: 12.3}, {Currency: "usd", ToppedUp: "8.8"}}
	got := formatDeepSeekBalance(items)
	if !strings.Contains(got, "CNY 12.3") || !strings.Contains(got, "USD 8.8") {
		t.Fatalf("unexpected: %s", got)
	}
	if formatBalanceForSummary("CNY 17.17 | USD 1") != "17.17 元" {
		t.Fatalf("summary cny mismatch")
	}
	if formatBalanceForSummary("USD 1") != "USD 1" {
		t.Fatalf("summary fallback mismatch")
	}
	if anyToString(3) != "3" || anyToString(3.2) != "3.2" {
		t.Fatalf("anyToString mismatch")
	}
	if firstNonEmpty("", "a") != "a" {
		t.Fatalf("firstNonEmpty mismatch")
	}
	if shortBody([]byte(strings.Repeat("a", 300))) == "" {
		t.Fatalf("shortBody empty")
	}
}

func TestFetchDeepSeekBalanceHTTP(t *testing.T) {
	orig := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = orig })

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" {
			http.NotFound(w, r)
			return
		}
		if strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer")) == "" {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"9.99"}]}`)
	}))
	defer ts.Close()
	targetURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	baseTransport := ts.Client().Transport
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = targetURL.Scheme
			clone.URL.Host = targetURL.Host
			return baseTransport.RoundTrip(clone)
		}),
	}

	balance, err := fetchDeepSeekBalance("abc")
	if err != nil {
		t.Fatalf("fetchDeepSeekBalance error: %v", err)
	}
	if balance != "CNY 9.99" {
		t.Fatalf("unexpected balance: %s", balance)
	}
}

func TestFetchDeepSeekBalanceWithRetryEmptyKey(t *testing.T) {
	_, err := fetchDeepSeekBalanceWithRetry("", 1)
	if err == nil {
		t.Fatalf("expected error")
	}
}
