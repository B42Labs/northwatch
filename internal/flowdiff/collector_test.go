package flowdiff

import (
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractDatapath(t *testing.T) {
	t.Run("nil row", func(t *testing.T) {
		assert.Equal(t, "", extractDatapath(nil))
	})

	t.Run("no logical_datapath key", func(t *testing.T) {
		assert.Equal(t, "", extractDatapath(map[string]any{"other": "value"}))
	})

	t.Run("string value", func(t *testing.T) {
		row := map[string]any{"logical_datapath": "dp-abc"}
		assert.Equal(t, "dp-abc", extractDatapath(row))
	})

	t.Run("string pointer value", func(t *testing.T) {
		dp := "dp-ptr"
		row := map[string]any{"logical_datapath": &dp}
		assert.Equal(t, "dp-ptr", extractDatapath(row))
	})

	t.Run("nil string pointer", func(t *testing.T) {
		row := map[string]any{"logical_datapath": (*string)(nil)}
		assert.Equal(t, "", extractDatapath(row))
	})

	t.Run("unexpected type", func(t *testing.T) {
		row := map[string]any{"logical_datapath": 42}
		assert.Equal(t, "", extractDatapath(row))
	})
}

func TestEventToFlowChange(t *testing.T) {
	t.Run("from new row", func(t *testing.T) {
		evt := events.Event{
			Type:  events.EventInsert,
			UUID:  "flow-1",
			Ts:    1000,
			Row:   map[string]any{"logical_datapath": "dp1"},
			Table: "Logical_Flow",
		}

		change := eventToFlowChange(evt)
		assert.Equal(t, int64(1000), change.Timestamp)
		assert.Equal(t, "insert", change.Type)
		assert.Equal(t, "flow-1", change.UUID)
		assert.Equal(t, "dp1", change.Datapath)
	})

	t.Run("fallback to old row", func(t *testing.T) {
		evt := events.Event{
			Type:   events.EventDelete,
			UUID:   "flow-2",
			Ts:     2000,
			OldRow: map[string]any{"logical_datapath": "dp2"},
		}

		change := eventToFlowChange(evt)
		assert.Equal(t, "dp2", change.Datapath)
		assert.Equal(t, "delete", change.Type)
	})
}

func TestStartCollector(t *testing.T) {
	hub := events.NewHub()
	store := NewStore(100, time.Hour)

	cleanup := StartCollector(hub, store)
	defer cleanup()

	hub.Publish(events.Event{
		Type:     events.EventInsert,
		Database: "sb",
		Table:    "Logical_Flow",
		UUID:     "f1",
		Ts:       time.Now().UnixMilli(),
		Row:      map[string]any{"logical_datapath": "dp1"},
	})

	// Give the goroutine time to process
	require.Eventually(t, func() bool {
		return len(store.Query("", 0)) == 1
	}, time.Second, 10*time.Millisecond)

	changes := store.Query("", 0)
	assert.Equal(t, "f1", changes[0].UUID)
	assert.Equal(t, "dp1", changes[0].Datapath)
}
