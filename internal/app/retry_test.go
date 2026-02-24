package app

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBackoffDurationAndJitter(t *testing.T) {
	d := backoffDuration(2, 100*time.Millisecond, 300*time.Millisecond, 0)
	if d != 200*time.Millisecond {
		t.Fatalf("unexpected delay: %v", d)
	}
	d2 := backoffDuration(10, 100*time.Millisecond, 300*time.Millisecond, 0)
	if d2 != 300*time.Millisecond {
		t.Fatalf("expected capped delay, got %v", d2)
	}
	j := applyJitter(100*time.Millisecond, 0)
	if j != 100*time.Millisecond {
		t.Fatalf("jitter=0 mismatch: %v", j)
	}
	if j2 := applyJitter(100*time.Millisecond, 2); j2 <= 0 {
		t.Fatalf("jitter clamp mismatch: %v", j2)
	}
	if d3 := backoffDuration(0, 100*time.Millisecond, 300*time.Millisecond, 0.2); d3 <= 0 {
		t.Fatalf("attempt clamp mismatch: %v", d3)
	}
}

func TestWithExponentialBackoff(t *testing.T) {
	attempts := 0
	err := withExponentialBackoff(retryOptions{MaxRetries: 0}, func(attempt int) error {
		attempts = attempt
		return nil
	})
	if err != nil || attempts != 1 {
		t.Fatalf("unexpected result err=%v attempts=%d", err, attempts)
	}

	attempts = 0
	err = withExponentialBackoff(retryOptions{MaxRetries: 0}, func(attempt int) error {
		attempts = attempt
		return errors.New("x")
	})
	if err == nil || attempts != 1 {
		t.Fatalf("expected immediate failure, err=%v attempts=%d", err, attempts)
	}
}

func TestWithExponentialBackoffOnRetry(t *testing.T) {
	var (
		retryCalled bool
		attempts    int
	)
	err := withExponentialBackoff(retryOptions{
		MaxRetries: 1,
		BaseDelay:  0,
		MaxDelay:   time.Millisecond,
		Jitter:     0,
		OnRetry: func(attempt int, wait time.Duration, err error) {
			retryCalled = true
			if attempt != 1 || wait < 0 || err == nil {
				t.Fatalf("unexpected retry callback args")
			}
		},
	}, func(attempt int) error {
		attempts = attempt
		if attempt == 1 {
			return errors.New("x")
		}
		return nil
	})
	if err != nil || !retryCalled || attempts != 2 {
		t.Fatalf("unexpected retry result: err=%v retryCalled=%v attempts=%d", err, retryCalled, attempts)
	}
}

func TestWithExponentialBackoffRateLimitBranch(t *testing.T) {
	start := time.Now()
	attempts := 0
	err := withExponentialBackoff(retryOptions{
		MaxRetries: 1,
		OnRetry:    func(attempt int, wait time.Duration, err error) {},
	}, func(attempt int) error {
		attempts = attempt
		if attempt == 1 {
			return errors.New("HTTP 429 rate limit")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("expected eventual success, err=%v attempts=%d", err, attempts)
	}
	if time.Since(start) < 600*time.Millisecond {
		t.Fatalf("rate-limit branch should wait noticeably before retry")
	}
}

func TestIsRateLimitError(t *testing.T) {
	if !isRateLimitError(errors.New("HTTP 429")) {
		t.Fatalf("expected true")
	}
	if !isRateLimitError(errors.New("RATE LIMIT")) {
		t.Fatalf("expected true")
	}
	if isRateLimitError(errors.New("bad")) {
		t.Fatalf("expected false")
	}
	if isRateLimitError(nil) {
		t.Fatalf("expected false")
	}
	_ = strings.Contains
}
