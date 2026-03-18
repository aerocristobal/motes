package dream

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_UnlimitedPassesThrough(t *testing.T) {
	rl := NewRateLimiter(0)
	for i := 0; i < 100; i++ {
		if err := rl.Wait(context.Background()); err != nil {
			t.Fatalf("unlimited limiter should never error: %v", err)
		}
	}
}

func TestRateLimiter_NegativeRPMIsUnlimited(t *testing.T) {
	rl := NewRateLimiter(-5)
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("negative RPM should be unlimited: %v", err)
	}
}

func TestRateLimiter_ConsumesTokens(t *testing.T) {
	rl := NewRateLimiter(5) // 5 RPM
	// Should be able to consume all 5 tokens immediately
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := rl.Wait(ctx)
		cancel()
		if err != nil {
			t.Fatalf("token %d should be available: %v", i, err)
		}
	}
	// 6th should block (and timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err := rl.Wait(ctx)
	cancel()
	if err == nil {
		t.Fatal("expected timeout when tokens exhausted")
	}
}

func TestRateLimiter_RefillsTokens(t *testing.T) {
	rl := NewRateLimiter(600) // 600 RPM = 10 per second, interval = 100ms
	// Consume all tokens
	for i := 0; i < 600; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		rl.Wait(ctx)
		cancel()
	}
	// Wait long enough for a refill (interval = 100ms)
	time.Sleep(150 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err := rl.Wait(ctx)
	cancel()
	if err != nil {
		t.Fatalf("expected token after refill: %v", err)
	}
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	rl := NewRateLimiter(1) // 1 RPM
	// Consume the one token
	rl.Wait(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := rl.Wait(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
