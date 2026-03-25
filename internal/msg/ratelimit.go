package msg

import (
	"sync"
	"time"
)

// RateLimiter implements per-pubkey token bucket rate limiting.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64 // tokens per second
	capacity float64 // max burst
}

type bucket struct {
	tokens   float64
	lastTime time.Time
}

// NewRateLimiter creates a limiter with the given rate (tokens/sec) and burst capacity.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     rate,
		capacity: float64(burst),
	}
}

// Allow checks if the pubkey is allowed to send right now.
func (rl *RateLimiter) Allow(pubkey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[pubkey]
	if !ok {
		b = &bucket{tokens: rl.capacity, lastTime: now}
		rl.buckets[pubkey] = b
	}

	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastTime = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// Cleanup removes stale buckets (older than maxAge).
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for k, b := range rl.buckets {
		if b.lastTime.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}
