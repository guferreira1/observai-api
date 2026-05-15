// Package retry provides a small reusable retry loop with bounded exponential
// backoff and full jitter for outbound provider HTTP calls.
package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// Policy configures the retry loop.
//
// MaxAttempts counts the first attempt; a value of 3 will try once and retry
// up to two times. BaseDelay seeds the exponential backoff and MaxDelay caps
// the per-attempt sleep. Jitter is sampled from [0, exp) and added to the
// base delay so concurrent callers do not synchronize their retries.
type Policy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// Default returns a policy suitable for outbound provider calls.
func Default() Policy {
	return Policy{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
	}
}

// Operation is a retryable function.
type Operation func(attempt int) error

// Do executes operation honoring policy and ctx cancellation. The operation
// is retried only when isRetryable returns true for the produced error.
//
// Do returns the last operation error (which is also returned when ctx is
// cancelled during a sleep so callers can identify the underlying failure
// rather than the cancellation that interrupted backoff).
func Do(ctx context.Context, policy Policy, isRetryable func(error) bool, operation Operation) error {
	attempts := policy.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	if isRetryable == nil {
		isRetryable = func(error) bool { return false }
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}

		err := operation(attempt)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt == attempts || !isRetryable(err) {
			return err
		}

		timer := time.NewTimer(backoff(policy, attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
	if lastErr == nil {
		lastErr = errors.New("retry: no attempts executed")
	}
	return lastErr
}

func backoff(policy Policy, attempt int) time.Duration {
	base := policy.BaseDelay
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	delay := policy.MaxDelay
	if delay <= 0 {
		delay = 2 * time.Second
	}

	scale := time.Duration(1) << uint(attempt-1)
	exp := base * scale
	if exp <= 0 || exp > delay {
		exp = delay
	}
	jitter := time.Duration(rand.Int64N(int64(exp)))
	return base + jitter
}
