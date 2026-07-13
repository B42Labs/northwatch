package write

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlanCacheStartCleanupSweep verifies the periodic cleanup loop physically
// removes an expired plan from the cache (not just hides it behind Get's
// expiry check). The clock is driven via the now seam so no sleeping is needed
// to reach expiry.
func TestPlanCacheStartCleanupSweep(t *testing.T) {
	cache := NewPlanCache(time.Hour)
	base := time.Now()
	cache.now = func() time.Time { return base }
	cache.Store(&Plan{ID: "sweep-me", Status: "pending"})

	// Advance the clock past the TTL so the next sweep considers it expired.
	cache.now = func() time.Time { return base.Add(2 * time.Hour) }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cache.StartCleanup(ctx, time.Millisecond)

	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return len(cache.plans) == 0
	}, 2*time.Second, 5*time.Millisecond, "cleanup loop must sweep the expired plan")
}

// TestRateLimiterRefill verifies the token bucket refills once enough wall-clock
// time has elapsed. lastRefill is backdated so the elapsed branch runs
// deterministically without waiting.
func TestRateLimiterRefill(t *testing.T) {
	rl := newRateLimiter(2)
	require.True(t, rl.allow())
	require.True(t, rl.allow())
	require.False(t, rl.allow(), "bucket should be drained")

	// Backdate the last refill by a full minute so a full bucket's worth of
	// tokens is credited on the next call.
	rl.mu.Lock()
	rl.lastRefill = time.Now().Add(-time.Minute)
	rl.mu.Unlock()

	assert.True(t, rl.allow(), "tokens should be replenished after the refill window")
}

// TestMapToModelFieldErrors covers the type-conversion error arms of
// setField/setSliceField/setMapField.
func TestMapToModelFieldErrors(t *testing.T) {
	lsType := reflect.TypeOf(nb.LogicalSwitch{})
	aclType := reflect.TypeOf(nb.ACL{})

	tests := []struct {
		name      string
		modelType reflect.Type
		fields    map[string]any
		wantErr   string
	}{
		{
			name:      "int column with non-number",
			modelType: aclType,
			fields:    map[string]any{"priority": "high"},
			wantErr:   "expected number",
		},
		{
			name:      "bool column with non-bool",
			modelType: aclType,
			fields:    map[string]any{"log": "yes"},
			wantErr:   "expected bool",
		},
		{
			name:      "slice column with non-slice",
			modelType: lsType,
			fields:    map[string]any{"acls": "not-a-slice"},
			wantErr:   "expected slice",
		},
		{
			name:      "slice element with wrong type",
			modelType: lsType,
			fields:    map[string]any{"acls": []any{42}},
			wantErr:   "expected string",
		},
		{
			name:      "map column with non-map",
			modelType: lsType,
			fields:    map[string]any{"external_ids": "not-a-map"},
			wantErr:   "expected map",
		},
		{
			name:      "map value with wrong type",
			modelType: lsType,
			fields:    map[string]any{"external_ids": map[string]any{"k": 42}},
			wantErr:   "expected string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := MapToModel(tc.fields, tc.modelType)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
