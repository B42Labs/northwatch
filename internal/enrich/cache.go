package enrich

import (
	"sync"
	"time"
)

// maxCacheEntries bounds the number of entries the cache holds so that
// churned keys cannot grow the map without limit.
const maxCacheEntries = 10000

type cacheEntry struct {
	info      *Info
	expiresAt time.Time
}

// Cache is a TTL-based in-memory cache for enrichment results.
//
// The cache is bounded to maxCacheEntries entries. When a new key is inserted
// while at capacity, expired entries are swept first, and if that does not free
// enough room, arbitrary entries are evicted so the map never exceeds the cap.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	// now returns the current time; a field so tests can drive expiry
	// deterministically. It defaults to time.Now and is read under mu.
	now func() time.Time
}

// NewCache creates a new Cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		now:     time.Now,
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
	if c.now().After(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		// Re-check under write lock to avoid racing with another goroutine.
		if e, exists := c.entries[key]; exists && c.now().After(e.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return nil, false
	}
	c.mu.RUnlock()
	return entry.info, true
}

// Set stores an Info in the cache with the configured TTL. Inserting a new key
// while the cache is at capacity first sweeps expired entries and, if still
// full, evicts arbitrary entries so the map never exceeds maxCacheEntries.
func (c *Cache) Set(key string, info *Info) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists && len(c.entries) >= maxCacheEntries {
		c.evictLocked()
	}

	c.entries[key] = cacheEntry{
		info:      info,
		expiresAt: c.now().Add(c.ttl),
	}
}

// evictLocked makes room for at least one new entry. It first removes all
// expired entries and, if the map is still at capacity, evicts arbitrary
// entries until it is below the cap. The caller must hold c.mu.
func (c *Cache) evictLocked() {
	now := c.now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
	for k := range c.entries {
		if len(c.entries) < maxCacheEntries {
			break
		}
		delete(c.entries, k)
	}
}
