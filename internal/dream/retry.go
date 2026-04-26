// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"strings"
	"time"
)

// RetryPolicy defines retry behavior with exponential backoff.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Retryable   func(err error) bool
}

// DefaultRetryPolicy returns a policy for transient LLM/API errors.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    4 * time.Second,
		Retryable:   IsTransientError,
	}
}

// Do executes fn with retries according to the policy.
func Do[T any](rp *RetryPolicy, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < rp.MaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !rp.Retryable(err) {
			return zero, err
		}
		if attempt < rp.MaxAttempts-1 {
			delay := rp.backoff(attempt)
			time.Sleep(delay)
		}
	}
	return zero, lastErr
}

func (rp *RetryPolicy) backoff(attempt int) time.Duration {
	delay := rp.BaseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	if delay > rp.MaxDelay {
		delay = rp.MaxDelay
	}
	return delay
}

// transientIndicators are substrings in error messages that indicate retryable failures.
var transientIndicators = []string{"429", "500", "502", "503", "rate limit", "timeout"}

// nonRetryableIndicators are substrings that indicate permanent failures.
// Includes Gemini safety/recitation blocks — retrying won't change the verdict.
var nonRetryableIndicators = []string{"400", "401", "403", "404", "gemini response blocked"}

// IsTransientError returns true if the error looks like a transient API failure.
func IsTransientError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, ind := range nonRetryableIndicators {
		if strings.Contains(msg, ind) {
			return false
		}
	}
	for _, ind := range transientIndicators {
		if strings.Contains(msg, ind) {
			return true
		}
	}
	// Default: don't retry unknown errors
	return false
}
