package write

import (
	"sync"
	"time"
)

// rateLimiter implements a simple token-bucket rate limiter.
type rateLimiter struct {
	mu         sync.Mutex
	tokens     int
	maxTokens  int
	refillRate int // tokens per minute
	lastRefill time.Time
}

func newRateLimiter(opsPerMinute int) *rateLimiter {
	return &rateLimiter{
		tokens:     opsPerMinute,
		maxTokens:  opsPerMinute,
		refillRate: opsPerMinute,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	if elapsed > 0 {
		refill := int(elapsed.Minutes() * float64(rl.refillRate))
		if refill > 0 {
			rl.tokens += refill
			if rl.tokens > rl.maxTokens {
				rl.tokens = rl.maxTokens
			}
			rl.lastRefill = now
		}
	}

	if rl.tokens <= 0 {
		return false
	}
	rl.tokens--
	return true
}
