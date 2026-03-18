package dream

import (
	"context"
	"sync"
	"time"
)

// RateLimiter implements a token-bucket rate limiter for LLM API calls.
type RateLimiter struct {
	mu       sync.Mutex
	tokens   int
	max      int
	interval time.Duration // time per token refill
	last     time.Time
}

// NewRateLimiter creates a rate limiter that allows rpm requests per minute.
// If rpm <= 0, returns a no-op limiter (unlimited).
func NewRateLimiter(rpm int) *RateLimiter {
	if rpm <= 0 {
		return &RateLimiter{max: 0}
	}
	return &RateLimiter{
		tokens:   rpm,
		max:      rpm,
		interval: time.Minute / time.Duration(rpm),
		last:     time.Now(),
	}
}

// Wait blocks until a token is available or the context is cancelled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	if rl.max == 0 {
		return nil // unlimited
	}
	for {
		rl.mu.Lock()
		rl.refill()
		if rl.tokens > 0 {
			rl.tokens--
			rl.mu.Unlock()
			return nil
		}
		// Calculate wait time for next token
		waitTime := rl.interval - time.Since(rl.last)
		if waitTime < 0 {
			waitTime = time.Millisecond
		}
		rl.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}
}

// refill adds tokens based on elapsed time. Must be called with mu held.
func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.last)
	newTokens := int(elapsed / rl.interval)
	if newTokens > 0 {
		rl.tokens += newTokens
		if rl.tokens > rl.max {
			rl.tokens = rl.max
		}
		rl.last = now
	}
}
