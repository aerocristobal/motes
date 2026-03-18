package dream

import (
	"fmt"
	"testing"
	"time"
)

func TestRetryPolicy_SucceedsFirstAttempt(t *testing.T) {
	rp := &RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}
	calls := 0
	result, err := rp.Do(func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected 'ok', got %q", result)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryPolicy_RetriesTransientThenSucceeds(t *testing.T) {
	rp := &RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}
	calls := 0
	result, err := rp.Do(func() (string, error) {
		calls++
		if calls < 3 {
			return "", fmt.Errorf("claude invocation failed: 429 rate limited")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected 'ok', got %q", result)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryPolicy_NonRetryableFailsImmediately(t *testing.T) {
	rp := &RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}
	calls := 0
	_, err := rp.Do(func() (string, error) {
		calls++
		return "", fmt.Errorf("claude invocation failed: 401 unauthorized")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call for non-retryable error, got %d", calls)
	}
}

func TestRetryPolicy_ExhaustsAttempts(t *testing.T) {
	rp := &RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}
	calls := 0
	_, err := rp.Do(func() (string, error) {
		calls++
		return "", fmt.Errorf("claude invocation failed: 503 service unavailable")
	})
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryPolicy_BackoffCapped(t *testing.T) {
	rp := &RetryPolicy{MaxAttempts: 5, BaseDelay: time.Second, MaxDelay: 4 * time.Second}
	if d := rp.backoff(0); d != time.Second {
		t.Errorf("attempt 0: expected 1s, got %v", d)
	}
	if d := rp.backoff(1); d != 2*time.Second {
		t.Errorf("attempt 1: expected 2s, got %v", d)
	}
	if d := rp.backoff(2); d != 4*time.Second {
		t.Errorf("attempt 2: expected 4s, got %v", d)
	}
	if d := rp.backoff(3); d != 4*time.Second {
		t.Errorf("attempt 3: expected 4s (capped), got %v", d)
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"429 too many requests", true},
		{"500 internal server error", true},
		{"502 bad gateway", true},
		{"503 service unavailable", true},
		{"rate limit exceeded", true},
		{"connection timeout", true},
		{"401 unauthorized", false},
		{"403 forbidden", false},
		{"404 not found", false},
		{"400 bad request", false},
		{"unknown error", false},
	}
	for _, tt := range tests {
		got := IsTransientError(fmt.Errorf("%s", tt.msg))
		if got != tt.want {
			t.Errorf("IsTransientError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}
