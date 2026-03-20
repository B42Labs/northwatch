package history

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneEventsByCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	var evts []EventRecord
	for i := 0; i < 10; i++ {
		evts = append(evts, EventRecord{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Type:      "insert",
			Database:  "nb",
			Table:     "Logical_Switch",
			UUID:      "uuid-" + string(rune('0'+i)),
		})
	}
	err := store.InsertEvents(ctx, evts)
	require.NoError(t, err)

	// Keep only 5 most recent
	pruned, err := store.PruneEventsByCount(ctx, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(5), pruned)

	remaining, err := store.QueryEvents(ctx, EventQueryOpts{})
	require.NoError(t, err)
	assert.Len(t, remaining, 5)
}

func TestPruneEventsByCount_Zero(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// maxCount=0 should not prune anything
	pruned, err := store.PruneEventsByCount(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pruned)
}

func TestEventCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	count, err := store.EventCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	err = store.InsertEvents(ctx, []EventRecord{
		{Timestamp: time.Now(), Type: "insert", Database: "nb", Table: "Logical_Switch", UUID: "uuid-1"},
		{Timestamp: time.Now(), Type: "insert", Database: "nb", Table: "Logical_Switch", UUID: "uuid-2"},
	})
	require.NoError(t, err)

	count, err = store.EventCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestSnapshotDeduplication(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-2", Data: map[string]any{"name": "sw2"}},
	}

	// Create first snapshot
	_, err := store.CreateSnapshot(ctx, "auto", "", rows)
	require.NoError(t, err)

	// Verify content hash is stored
	hash, err := store.LatestSnapshotContentHash(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Create second snapshot with same data — hash should match
	_, err = store.CreateSnapshot(ctx, "auto", "", rows)
	require.NoError(t, err)

	hash2, err := store.LatestSnapshotContentHash(ctx)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)
}

func TestSnapshotDeduplication_DifferentData(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows1 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	rows2 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1-changed"}},
	}

	_, err := store.CreateSnapshot(ctx, "auto", "", rows1)
	require.NoError(t, err)
	hash1, err := store.LatestSnapshotContentHash(ctx)
	require.NoError(t, err)

	_, err = store.CreateSnapshot(ctx, "auto", "", rows2)
	require.NoError(t, err)
	hash2, err := store.LatestSnapshotContentHash(ctx)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2)
}

func TestCollector_TakeSnapshotIfChanged(t *testing.T) {
	store := newTestStore(t)
	hub := events.NewHub()
	ctx := context.Background()

	sources := []TableSource{
		{
			Database: "nb",
			Table:    "Logical_Switch",
			ListFunc: func(ctx context.Context) ([]map[string]any, error) {
				return []map[string]any{
					{"_uuid": "uuid-1", "name": "sw1"},
				}, nil
			},
		},
	}

	collector := NewCollector(store, hub, sources, 5*time.Minute, 24*time.Hour)

	// First snapshot should succeed
	meta, err := collector.TakeSnapshotIfChanged(ctx, "auto", "")
	require.NoError(t, err)
	require.NotNil(t, meta)

	// Second snapshot with identical data should be skipped
	meta2, err := collector.TakeSnapshotIfChanged(ctx, "auto", "")
	require.NoError(t, err)
	assert.Nil(t, meta2, "should skip snapshot when data hasn't changed")

	// Verify only one snapshot exists
	list, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestLatestSnapshotContentHash_Empty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	hash, err := store.LatestSnapshotContentHash(ctx)
	require.NoError(t, err)
	assert.Empty(t, hash)
}

func TestComputeContentHash_Deterministic(t *testing.T) {
	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-2", Data: map[string]any{"name": "sw2"}},
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	// Different order
	rows2 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-2", Data: map[string]any{"name": "sw2"}},
	}

	assert.Equal(t, computeContentHash(rows), computeContentHash(rows2))
}

func TestComputeContentHash_Empty(t *testing.T) {
	hash := computeContentHash(nil)
	assert.NotEmpty(t, hash) // SHA-256 of empty input is still a hash
}
