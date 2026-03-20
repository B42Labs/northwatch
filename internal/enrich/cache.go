package enrich

import (
	"sync"
	"time"
)

type cacheEntry struct {
	info      *Info
	expiresAt time.Time
}

// Cache is a TTL-based in-memory cache for enrichment results.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

// NewCache creates a new Cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// Get returns the cached Info for the given key, or false if not found or expired.
// Expired entries are evicted on access.
func (c *Cache) Get(key string) (*Info, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		// Re-check under write lock to avoid racing with another goroutine.
		if e, exists := c.entries[key]; exists && time.Now().After(e.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return nil, false
	}
	c.mu.RUnlock()
	return entry.info, true
}

// Set stores an Info in the cache with the configured TTL.
func (c *Cache) Set(key string, info *Info) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		info:      info,
		expiresAt: time.Now().Add(c.ttl),
	}
}
