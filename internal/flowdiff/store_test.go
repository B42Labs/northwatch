package flowdiff

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_AddAndQuery(t *testing.T) {
	store := NewStore(100, time.Hour)

	now := time.Now().UnixMilli()
	store.Add(FlowChange{
		Timestamp: now - 3000,
		Type:      "insert",
		UUID:      "f1",
		Datapath:  "dp1",
	})
	store.Add(FlowChange{
		Timestamp: now - 2000,
		Type:      "update",
		UUID:      "f2",
		Datapath:  "dp2",
	})
	store.Add(FlowChange{
		Timestamp: now - 1000,
		Type:      "delete",
		UUID:      "f3",
		Datapath:  "dp1",
	})

	// Query all
	all := store.Query("", 0)
	require.Len(t, all, 3)

	// Filter by datapath
	dp1 := store.Query("dp1", 0)
	require.Len(t, dp1, 2)
	assert.Equal(t, "f1", dp1[0].UUID)
	assert.Equal(t, "f3", dp1[1].UUID)

	// Filter by since
	recent := store.Query("", now-2000)
	require.Len(t, recent, 2)
	assert.Equal(t, "f2", recent[0].UUID)
	assert.Equal(t, "f3", recent[1].UUID)

	// Combined filter
	dp1Recent := store.Query("dp1", now-1500)
	require.Len(t, dp1Recent, 1)
	assert.Equal(t, "f3", dp1Recent[0].UUID)
}

func TestStore_MaxSizePruning(t *testing.T) {
	store := NewStore(3, time.Hour)

	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		store.Add(FlowChange{
			Timestamp: now + int64(i*1000),
			Type:      "insert",
			UUID:      "f" + string(rune('0'+i)),
			Datapath:  "dp",
		})
	}

	all := store.Query("", 0)
	require.Len(t, all, 3, "should prune to maxSize")
	// Should keep the most recent entries
	assert.Equal(t, now+int64(2000), all[0].Timestamp)
	assert.Equal(t, now+int64(4000), all[2].Timestamp)
}

func TestStore_MaxAgePruning(t *testing.T) {
	store := NewStore(100, 100*time.Millisecond)

	store.Add(FlowChange{
		Timestamp: time.Now().Add(-200 * time.Millisecond).UnixMilli(),
		Type:      "insert",
		UUID:      "old",
		Datapath:  "dp",
	})

	store.Add(FlowChange{
		Timestamp: time.Now().UnixMilli(),
		Type:      "insert",
		UUID:      "new",
		Datapath:  "dp",
	})

	all := store.Query("", 0)
	require.Len(t, all, 1, "old entry should be pruned")
	assert.Equal(t, "new", all[0].UUID)
}

func TestStore_EmptyQuery(t *testing.T) {
	store := NewStore(100, time.Hour)
	all := store.Query("", 0)
	assert.Nil(t, all)
}

func TestStore_ConcurrentAccess(t *testing.T) {
	store := NewStore(1000, time.Hour)

	var wg sync.WaitGroup
	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				store.Add(FlowChange{
					Timestamp: time.Now().UnixMilli(),
					Type:      "insert",
					UUID:      "f",
					Datapath:  "dp",
				})
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = store.Query("", 0)
			}
		}()
	}

	wg.Wait()
	// Just verify no panics/races
	all := store.Query("", 0)
	assert.NotNil(t, all)
}
