package app

import (
	"math/rand"
	"strings"
	"time"
)

type retryOptions struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Jitter     float64
	OnRetry    func(attempt int, wait time.Duration, err error)
}

func withExponentialBackoff(opts retryOptions, fn func(attempt int) error) error {
	attempts := opts.MaxRetries + 1
	if attempts <= 0 {
		attempts = 1
	}
	base := opts.BaseDelay
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	maxDelay := opts.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 8 * time.Second
	}
	jitter := opts.Jitter
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := fn(attempt); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == attempts {
			break
		}
		wait := backoffDuration(attempt, base, maxDelay, jitter)
		if isRateLimitError(lastErr) {
			rateLimitWait := time.Duration(attempt*attempt) * time.Second
			if wait < rateLimitWait {
				wait = rateLimitWait
			}
			wait = applyJitter(wait, 0.35)
			if wait > 60*time.Second {
				wait = 60 * time.Second
			}
		}
		if opts.OnRetry != nil {
			opts.OnRetry(attempt, wait, lastErr)
		}
		time.Sleep(wait)
	}
	return lastErr
}

func backoffDuration(attempt int, base, maxDelay time.Duration, jitter float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 30 {
		shift = 30
	}
	delay := base << shift
	if delay > maxDelay || delay < 0 {
		delay = maxDelay
	}
	if jitter == 0 {
		return delay
	}
	out := applyJitter(delay, jitter)
	if out < 0 {
		return 0
	}
	return out
}

func applyJitter(delay time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return delay
	}
	if jitter > 1 {
		jitter = 1
	}
	low := 1 - jitter
	high := 1 + jitter
	factor := low + rand.Float64()*(high-low)
	return time.Duration(float64(delay) * factor)
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") || strings.Contains(s, "rate limit")
}
