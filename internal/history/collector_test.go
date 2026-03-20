package history

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_TakeSnapshot(t *testing.T) {
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
					{"_uuid": "uuid-2", "name": "sw2"},
				}, nil
			},
		},
		{
			Database: "sb",
			Table:    "Chassis",
			ListFunc: func(ctx context.Context) ([]map[string]any, error) {
				return []map[string]any{
					{"_uuid": "uuid-3", "hostname": "node1"},
				}, nil
			},
		},
	}

	collector := NewCollector(store, hub, sources, 5*time.Minute, 24*time.Hour)
	meta, err := collector.TakeSnapshot(ctx, "manual", "test")
	require.NoError(t, err)

	assert.Equal(t, "manual", meta.Trigger)
	assert.Equal(t, "test", meta.Label)
	assert.Equal(t, 2, meta.RowCounts["nb.Logical_Switch"])
	assert.Equal(t, 1, meta.RowCounts["sb.Chassis"])

	// Verify rows are stored correctly
	rows, err := store.GetSnapshotRows(ctx, meta.ID, "nb", "Logical_Switch")
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestCollector_EventPersistence(t *testing.T) {
	store := newTestStore(t)
	hub := events.NewHub()
	ctx := context.Background()

	collector := NewCollector(store, hub, nil, 1*time.Hour, 24*time.Hour)
	stop := collector.Start(ctx)
	defer stop()

	// Publish some events
	hub.Publish(events.Event{
		Type:     events.EventInsert,
		Database: "nb",
		Table:    "Logical_Switch",
		UUID:     "uuid-1",
		Row:      map[string]any{"name": "sw1"},
		Ts:       time.Now().UnixMilli(),
	})
	hub.Publish(events.Event{
		Type:     events.EventUpdate,
		Database: "nb",
		Table:    "Logical_Switch",
		UUID:     "uuid-1",
		Row:      map[string]any{"name": "sw1-v2"},
		OldRow:   map[string]any{"name": "sw1"},
		Ts:       time.Now().UnixMilli(),
	})

	// Wait for batch flush
	time.Sleep(300 * time.Millisecond)

	got, err := store.QueryEvents(ctx, EventQueryOpts{})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
