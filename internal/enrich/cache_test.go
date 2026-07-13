package enrich

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetSet(t *testing.T) {
	c := NewCache(5 * time.Minute)

	info := &Info{DisplayName: "test-port"}
	c.Set("port:abc", info)

	got, ok := c.Get("port:abc")
	require.True(t, ok)
	assert.Equal(t, "test-port", got.DisplayName)
}

func TestCache_Miss(t *testing.T) {
	c := NewCache(5 * time.Minute)

	_, ok := c.Get("nonexistent")
	assert.False(t, ok)
}

func TestCache_Expiration(t *testing.T) {
	c := NewCache(1 * time.Minute)
	base := time.Now()
	current := base
	c.now = func() time.Time { return current }

	c.Set("key", &Info{DisplayName: "expiring"})

	_, ok := c.Get("key")
	require.True(t, ok)

	// Advance past the TTL instead of sleeping.
	current = base.Add(2 * time.Minute)
	_, ok = c.Get("key")
	assert.False(t, ok)
}

func TestCache_Overwrite(t *testing.T) {
	c := NewCache(5 * time.Minute)

	c.Set("key", &Info{DisplayName: "first"})
	c.Set("key", &Info{DisplayName: "second"})

	got, ok := c.Get("key")
	require.True(t, ok)
	assert.Equal(t, "second", got.DisplayName)
}

func TestCache_BoundedSize(t *testing.T) {
	c := NewCache(5 * time.Minute)

	// Insert well beyond the cap with distinct keys.
	for i := 0; i < maxCacheEntries+500; i++ {
		c.Set(fmt.Sprintf("key:%d", i), &Info{DisplayName: "value"})
	}

	c.mu.RLock()
	got := len(c.entries)
	c.mu.RUnlock()
	assert.LessOrEqual(t, got, maxCacheEntries)
}

func TestCache_ExpiredEvictedFirst(t *testing.T) {
	c := NewCache(5 * time.Minute)

	// Fill to capacity with already-expired entries.
	past := time.Now().Add(-time.Hour)
	c.mu.Lock()
	for i := 0; i < maxCacheEntries; i++ {
		c.entries[fmt.Sprintf("stale:%d", i)] = cacheEntry{
			info:      &Info{DisplayName: "stale"},
			expiresAt: past,
		}
	}
	c.mu.Unlock()

	// Inserting a new key at capacity should sweep the expired entries first.
	c.Set("fresh", &Info{DisplayName: "fresh"})

	c.mu.RLock()
	got := len(c.entries)
	_, staleStillPresent := c.entries["stale:0"]
	c.mu.RUnlock()

	assert.LessOrEqual(t, got, maxCacheEntries)
	assert.False(t, staleStillPresent, "expired entries should have been swept")

	info, ok := c.Get("fresh")
	require.True(t, ok, "fresh key must be retrievable after eviction")
	assert.Equal(t, "fresh", info.DisplayName)
}

func TestCache_WithinTTLRetrievableAtCapacity(t *testing.T) {
	c := NewCache(5 * time.Minute)

	c.Set("live", &Info{DisplayName: "live"})

	// Push the cache to capacity with additional distinct live keys.
	for i := 0; i < maxCacheEntries; i++ {
		c.Set(fmt.Sprintf("filler:%d", i), &Info{DisplayName: "filler"})
	}

	// Overwriting the existing key must never trigger eviction of itself.
	c.Set("live", &Info{DisplayName: "live-updated"})
	info, ok := c.Get("live")
	require.True(t, ok)
	assert.Equal(t, "live-updated", info.DisplayName)
}

func TestCache_Concurrent(t *testing.T) {
	c := NewCache(5 * time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.Set("key", &Info{DisplayName: "value"})
		}()
		go func() {
			defer wg.Done()
			c.Get("key")
		}()
	}

	wg.Wait()
}
