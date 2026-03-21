package write

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_AllowsUpToLimit(t *testing.T) {
	rl := newRateLimiter(3)
	assert.True(t, rl.allow())
	assert.True(t, rl.allow())
	assert.True(t, rl.allow())
	assert.False(t, rl.allow(), "should be exhausted")
}

func TestRateLimiter_ZeroLimit(t *testing.T) {
	// A nil rateLimiter means unlimited — tested via engine logic.
	// A limiter with 0 tokens should never allow.
	rl := newRateLimiter(0)
	assert.False(t, rl.allow())
}
