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

func TestCollector_PausedSkipsAutoSnapshot(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	sources := []TableSource{{
		Database: "nb",
		Table:    "Logical_Switch",
		ListFunc: func(ctx context.Context) ([]map[string]any, error) {
			return []map[string]any{{"_uuid": "uuid-1", "name": "sw1"}}, nil
		},
	}}
	collector := NewCollector(store, events.NewHub(), sources, 5*time.Minute, 24*time.Hour)

	paused := true
	collector.SetPauseCheck(func() bool { return paused })

	meta, err := collector.TakeSnapshotIfChanged(ctx, "auto", "")
	require.NoError(t, err)
	assert.Nil(t, meta, "no snapshot should be taken while paused")

	paused = false
	meta, err = collector.TakeSnapshotIfChanged(ctx, "auto", "")
	require.NoError(t, err)
	require.NotNil(t, meta, "snapshot should be taken once unpaused")
	assert.Equal(t, 1, meta.RowCounts["nb.Logical_Switch"])
}

func TestCollector_PausedDropsEvents(t *testing.T) {
	store := newTestStore(t)
	hub := events.NewHub()
	ctx := context.Background()

	collector := NewCollector(store, hub, nil, 1*time.Hour, 24*time.Hour)
	paused := true
	collector.SetPauseCheck(func() bool { return paused })
	stop := collector.Start(ctx)
	defer stop()

	hub.Publish(events.Event{
		Type:     events.EventInsert,
		Database: "nb",
		Table:    "Logical_Switch",
		UUID:     "dropped",
		Row:      map[string]any{"name": "sw1"},
		Ts:       time.Now().UnixMilli(),
	})

	// While paused the event must not be persisted.
	require.Never(t, func() bool {
		got, err := store.QueryEvents(ctx, EventQueryOpts{})
		return err == nil && len(got) > 0
	}, 300*time.Millisecond, 20*time.Millisecond)
}

func TestCollector_EventPersistence(t *testing.T) {
	store := newTestStore(t)
	hub := events.NewHub()
	ctx := context.Background()

	collector := NewCollector(store, hub, nil, 1*time.Hour, 24*time.Hour)
	stop := collector.Start(ctx)

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

	// stop() drains the subscriber buffer, flushes, and joins the goroutines,
	// so both events are durably persisted by the time it returns — no sleep.
	stop()

	got, err := store.QueryEvents(ctx, EventQueryOpts{})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

// TestCollector_StopWaitsForFlush asserts that stop() drains and persists a
// batch that is still below the size threshold and never hit the flush timer,
// so a graceful shutdown cannot lose the final events (the flush completes
// before the caller closes the store).
func TestCollector_StopWaitsForFlush(t *testing.T) {
	store := newTestStore(t)
	hub := events.NewHub()
	ctx := context.Background()

	// A long interval means the 100ms flush timer is the only steady-state
	// flush path; we stop before it fires to prove stop() itself flushes.
	collector := NewCollector(store, hub, nil, 1*time.Hour, 24*time.Hour)
	stop := collector.Start(ctx)

	hub.Publish(events.Event{
		Type:     events.EventInsert,
		Database: "sb",
		Table:    "Chassis",
		UUID:     "chassis-1",
		Row:      map[string]any{"name": "node1"},
		Ts:       time.Now().UnixMilli(),
	})

	stop()

	// The store is still open here; querying it immediately after stop() must
	// already see the flushed event.
	got, err := store.QueryEvents(ctx, EventQueryOpts{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "chassis-1", got[0].UUID)
}
