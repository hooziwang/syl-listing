package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func withPatchedDeepSeekBalanceEndpoint(t *testing.T, handler http.HandlerFunc) {
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

func TestFetchDeepSeekBalanceErrorBranches(t *testing.T) {
	withPatchedDeepSeekBalanceEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("x", 350), http.StatusInternalServerError)
	})
	if _, err := fetchDeepSeekBalance("k"); err == nil || !strings.Contains(err.Error(), "余额接口返回 500") {
		t.Fatalf("expected http status error, got %v", err)
	}

	withPatchedDeepSeekBalanceEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{bad-json"))
	})
	if _, err := fetchDeepSeekBalance("k"); err == nil || !strings.Contains(err.Error(), "解析余额响应失败") {
		t.Fatalf("expected json parse error, got %v", err)
	}

	withPatchedDeepSeekBalanceEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":{"message":"bad request"}}`)
	})
	if _, err := fetchDeepSeekBalance("k"); err == nil || !strings.Contains(err.Error(), "余额接口错误") {
		t.Fatalf("expected api error field, got %v", err)
	}

	withPatchedDeepSeekBalanceEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"is_available":true,"balance_infos":[]}`)
	})
	if _, err := fetchDeepSeekBalance("k"); err == nil || !strings.Contains(err.Error(), "余额接口返回为空") {
		t.Fatalf("expected empty balance error, got %v", err)
	}
}

func TestFetchDeepSeekBalanceWithRetryEventuallySuccess(t *testing.T) {
	calls := 0
	withPatchedDeepSeekBalanceEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "temporary", http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"7.77"}]}`)
	})
	got, err := fetchDeepSeekBalanceWithRetry("k", 1)
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if got != "CNY 7.77" || calls < 2 {
		t.Fatalf("unexpected retry result: got=%q calls=%d", got, calls)
	}
}

func TestBalanceFormatHelpersExtraBranches(t *testing.T) {
	if got := formatDeepSeekBalance(nil); got != "" {
		t.Fatalf("nil list should be empty, got %q", got)
	}
	items := []deepSeekBalanceInfo{
		{Currency: "", TotalBalance: ""},
		{Currency: "", GrantedAmount: "3.3"},
	}
	got := formatDeepSeekBalance(items)
	if !strings.Contains(got, "UNKNOWN 3.3") {
		t.Fatalf("expected UNKNOWN fallback, got %q", got)
	}

	if got := firstNonEmpty("", "  "); got != "" {
		t.Fatalf("all empty should return empty, got %q", got)
	}
	if got := shortBody([]byte("")); got != "-" {
		t.Fatalf("empty shortBody should return -, got %q", got)
	}
	if got := shortBody([]byte("abc")); got != "abc" {
		t.Fatalf("short body should keep original, got %q", got)
	}
}
