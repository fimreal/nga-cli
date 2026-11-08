package limiter

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := New(50*time.Millisecond, 10*time.Millisecond)

	ctx := context.Background()

	// First call should not block much
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 40*time.Millisecond {
		t.Errorf("first call took too long: %v", time.Since(start))
	}

	// Second call immediately after should wait at least 50ms
	start = time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 45*time.Millisecond {
		t.Errorf("second call didn't wait enough: %v", elapsed)
	}

	// Test circuit breaker
	rl.RecordBlocked()
	rl.RecordBlocked()
	rl.RecordBlocked()

	err := rl.Wait(ctx)
	if err != ErrCircuitBreakerOpen {
		t.Errorf("expected ErrCircuitBreakerOpen, got %v", err)
	}

	// Reset
	rl.Reset()
	if err := rl.Wait(ctx); err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}
