package write

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanCache_StoreAndGet(t *testing.T) {
	cache := NewPlanCache(5 * time.Minute)

	plan := &Plan{
		ID:     "plan-1",
		Status: "pending",
	}
	cache.Store(plan)

	got, ok := cache.Get("plan-1")
	require.True(t, ok)
	assert.Equal(t, "plan-1", got.ID)
	assert.Equal(t, "pending", got.Status)
}

func TestPlanCache_GetMissing(t *testing.T) {
	cache := NewPlanCache(5 * time.Minute)

	_, ok := cache.Get("nonexistent")
	assert.False(t, ok)
}

func TestPlanCache_Expired(t *testing.T) {
	cache := NewPlanCache(1 * time.Millisecond)

	plan := &Plan{
		ID:     "plan-expired",
		Status: "pending",
	}
	cache.Store(plan)

	// Wait for the plan to expire.
	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("plan-expired")
	assert.False(t, ok)
}

func TestPlanCache_Delete(t *testing.T) {
	cache := NewPlanCache(5 * time.Minute)

	plan := &Plan{
		ID:     "plan-delete",
		Status: "pending",
	}
	cache.Store(plan)

	cache.Delete("plan-delete")

	_, ok := cache.Get("plan-delete")
	assert.False(t, ok)
}

func TestPlanCache_Cleanup(t *testing.T) {
	cache := NewPlanCache(1 * time.Millisecond)

	cache.Store(&Plan{ID: "plan-a", Status: "pending"})
	cache.Store(&Plan{ID: "plan-b", Status: "pending"})

	// Wait for plans to expire.
	time.Sleep(5 * time.Millisecond)

	cache.Cleanup()

	_, okA := cache.Get("plan-a")
	_, okB := cache.Get("plan-b")
	assert.False(t, okA)
	assert.False(t, okB)
}

func TestPlanCache_StoreUpdatesExpiry(t *testing.T) {
	cache := NewPlanCache(1 * time.Hour)

	plan := &Plan{
		ID:     "plan-ttl",
		Status: "pending",
	}
	cache.Store(plan)

	got, ok := cache.Get("plan-ttl")
	require.True(t, ok)
	// ExpiresAt should be approximately 1 hour from now.
	assert.True(t, got.ExpiresAt.After(time.Now().Add(59*time.Minute)))
}
