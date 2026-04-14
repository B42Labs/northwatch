package write

import (
	"context"
	"sync"
	"time"
)

// PlanCache is an in-memory TTL cache for pending plans.
type PlanCache struct {
	mu    sync.Mutex
	plans map[string]*Plan
	ttl   time.Duration
}

// NewPlanCache creates a new PlanCache with the given TTL for plans.
func NewPlanCache(ttl time.Duration) *PlanCache {
	return &PlanCache{
		plans: make(map[string]*Plan),
		ttl:   ttl,
	}
}

// Store adds a plan to the cache, setting its ExpiresAt to now + ttl.
func (c *PlanCache) Store(plan *Plan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	plan.ExpiresAt = time.Now().Add(c.ttl)
	c.plans[plan.ID] = plan
}

// Get retrieves a plan by ID. Returns false for expired or missing plans.
func (c *PlanCache) Get(id string) (*Plan, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	plan, ok := c.plans[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(plan.ExpiresAt) {
		plan.Status = "expired"
		delete(c.plans, id)
		return nil, false
	}
	return plan, true
}

// Delete removes a plan from the cache.
func (c *PlanCache) Delete(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.plans, id)
}

// Cleanup removes all expired plans from the cache.
func (c *PlanCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for id, plan := range c.plans {
		if now.After(plan.ExpiresAt) {
			delete(c.plans, id)
		}
	}
}

// StartCleanup runs periodic cleanup at the given interval until ctx is cancelled.
// It blocks and should be called in a goroutine.
func (c *PlanCache) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Cleanup()
		}
	}
}
