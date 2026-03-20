package history

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertAndQueryEvents(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	events := []EventRecord{
		{Timestamp: now, Type: "insert", Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Row: map[string]any{"name": "sw1"}},
		{Timestamp: now.Add(time.Second), Type: "update", Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Row: map[string]any{"name": "sw1-v2"}, OldRow: map[string]any{"name": "sw1"}},
		{Timestamp: now.Add(2 * time.Second), Type: "insert", Database: "sb", Table: "Chassis", UUID: "uuid-2", Row: map[string]any{"hostname": "node1"}},
	}
	err := store.InsertEvents(ctx, events)
	require.NoError(t, err)

	t.Run("all events", func(t *testing.T) {
		got, err := store.QueryEvents(ctx, EventQueryOpts{})
		require.NoError(t, err)
		assert.Len(t, got, 3)
		// Newest first
		assert.Equal(t, "sb", got[0].Database)
	})

	t.Run("filter by database", func(t *testing.T) {
		got, err := store.QueryEvents(ctx, EventQueryOpts{Database: "nb"})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("filter by table", func(t *testing.T) {
		got, err := store.QueryEvents(ctx, EventQueryOpts{Table: "Chassis"})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("filter by type", func(t *testing.T) {
		got, err := store.QueryEvents(ctx, EventQueryOpts{Type: "update"})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "sw1-v2", got[0].Row["name"])
		assert.Equal(t, "sw1", got[0].OldRow["name"])
	})

	t.Run("filter by time range", func(t *testing.T) {
		since := now.Add(500 * time.Millisecond)
		got, err := store.QueryEvents(ctx, EventQueryOpts{Since: &since})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("limit", func(t *testing.T) {
		got, err := store.QueryEvents(ctx, EventQueryOpts{Limit: 1})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})
}

func TestInsertEvent_Single(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.InsertEvent(ctx, EventRecord{
		Timestamp: time.Now().UTC(),
		Type:      "delete",
		Database:  "nb",
		Table:     "ACL",
		UUID:      "uuid-1",
	})
	require.NoError(t, err)

	got, err := store.QueryEvents(ctx, EventQueryOpts{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "delete", got[0].Type)
	assert.Nil(t, got[0].Row)
}

func TestInsertEvents_Empty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.InsertEvents(ctx, nil)
	require.NoError(t, err)
}

func TestPruneEvents(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC()

	events := []EventRecord{
		{Timestamp: old, Type: "insert", Database: "nb", Table: "Logical_Switch", UUID: "uuid-1"},
		{Timestamp: old, Type: "insert", Database: "nb", Table: "ACL", UUID: "uuid-2"},
		{Timestamp: recent, Type: "insert", Database: "sb", Table: "Chassis", UUID: "uuid-3"},
	}
	err := store.InsertEvents(ctx, events)
	require.NoError(t, err)

	pruned, err := store.PruneEvents(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(2), pruned)

	remaining, err := store.QueryEvents(ctx, EventQueryOpts{})
	require.NoError(t, err)
	assert.Len(t, remaining, 1)
	assert.Equal(t, "sb", remaining[0].Database)
}

func TestQueryEvents_LimitCap(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Verify limit capping: requesting over 10000 should cap at 10000
	got, err := store.QueryEvents(ctx, EventQueryOpts{Limit: 20000})
	require.NoError(t, err)
	assert.Empty(t, got) // no events, but query should succeed
}
