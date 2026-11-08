package limiter

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"
)

var (
	ErrCircuitBreakerOpen = errors.New("rate limiter: circuit breaker tripped due to consecutive WAF/429 challenges")
)

// RateLimiter manages pacing between outgoing requests to NGA.
type RateLimiter struct {
	mu           sync.Mutex
	minInterval  time.Duration
	maxJitter    time.Duration
	lastRequest  time.Time
	consecErrors int
	maxErrors    int
	rng          *rand.Rand
}

// New creates a new RateLimiter.
func New(minInterval time.Duration, maxJitter time.Duration) *RateLimiter {
	if minInterval <= 0 {
		minInterval = 1500 * time.Millisecond
	}
	if maxJitter < 0 {
		maxJitter = 500 * time.Millisecond
	}
	return &RateLimiter{
		minInterval: minInterval,
		maxJitter:   maxJitter,
		maxErrors:   3,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Wait blocks until the required cooldown and jitter delay have elapsed.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()

	if rl.consecErrors >= rl.maxErrors {
		rl.mu.Unlock()
		return ErrCircuitBreakerOpen
	}

	now := time.Now()
	elapsed := now.Sub(rl.lastRequest)
	var waitDuration time.Duration

	if rl.lastRequest.IsZero() {
		// First request: zero or minimal delay
		waitDuration = 0
	} else if elapsed < rl.minInterval {
		waitDuration = rl.minInterval - elapsed
		if rl.maxJitter > 0 {
			jitter := time.Duration(rl.rng.Int63n(int64(rl.maxJitter)))
			waitDuration += jitter
		}
	}

	rl.lastRequest = now.Add(waitDuration)
	rl.mu.Unlock()

	if waitDuration > 0 {
		select {
		case <-time.After(waitDuration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// RecordSuccess resets the consecutive error counter.
func (rl *RateLimiter) RecordSuccess() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.consecErrors = 0
}

// RecordBlocked records a WAF or rate limit error, incrementing the circuit counter.
func (rl *RateLimiter) RecordBlocked() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.consecErrors++
}

// Reset clears the error state.
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.consecErrors = 0
	rl.lastRequest = time.Time{}
}
