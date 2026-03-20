package history

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffSnapshots_Identical(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	snap1, err := store.CreateSnapshot(ctx, "manual", "", rows)
	require.NoError(t, err)
	snap2, err := store.CreateSnapshot(ctx, "manual", "", rows)
	require.NoError(t, err)

	diff, err := store.DiffSnapshots(ctx, snap1.ID, snap2.ID, "")
	require.NoError(t, err)
	assert.Empty(t, diff.Tables)
}

func TestDiffSnapshots_AddedRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows1 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	rows2 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-2", Data: map[string]any{"name": "sw2"}},
	}

	snap1, err := store.CreateSnapshot(ctx, "manual", "", rows1)
	require.NoError(t, err)
	snap2, err := store.CreateSnapshot(ctx, "manual", "", rows2)
	require.NoError(t, err)

	diff, err := store.DiffSnapshots(ctx, snap1.ID, snap2.ID, "")
	require.NoError(t, err)
	require.Len(t, diff.Tables, 1)
	assert.Len(t, diff.Tables[0].Added, 1)
	assert.Empty(t, diff.Tables[0].Removed)
	assert.Empty(t, diff.Tables[0].Modified)
}

func TestDiffSnapshots_RemovedRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows1 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-2", Data: map[string]any{"name": "sw2"}},
	}
	rows2 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}

	snap1, err := store.CreateSnapshot(ctx, "manual", "", rows1)
	require.NoError(t, err)
	snap2, err := store.CreateSnapshot(ctx, "manual", "", rows2)
	require.NoError(t, err)

	diff, err := store.DiffSnapshots(ctx, snap1.ID, snap2.ID, "")
	require.NoError(t, err)
	require.Len(t, diff.Tables, 1)
	assert.Empty(t, diff.Tables[0].Added)
	assert.Len(t, diff.Tables[0].Removed, 1)
	assert.Empty(t, diff.Tables[0].Modified)
}

func TestDiffSnapshots_ModifiedRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows1 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1", "ports": float64(3)}},
	}
	rows2 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1-renamed", "ports": float64(3)}},
	}

	snap1, err := store.CreateSnapshot(ctx, "manual", "", rows1)
	require.NoError(t, err)
	snap2, err := store.CreateSnapshot(ctx, "manual", "", rows2)
	require.NoError(t, err)

	diff, err := store.DiffSnapshots(ctx, snap1.ID, snap2.ID, "")
	require.NoError(t, err)
	require.Len(t, diff.Tables, 1)
	assert.Empty(t, diff.Tables[0].Added)
	assert.Empty(t, diff.Tables[0].Removed)
	require.Len(t, diff.Tables[0].Modified, 1)

	mod := diff.Tables[0].Modified[0]
	assert.Equal(t, "uuid-1", mod.UUID)
	require.Len(t, mod.Fields, 1)
	assert.Equal(t, "name", mod.Fields[0].Field)
	assert.Equal(t, "sw1", mod.Fields[0].OldValue)
	assert.Equal(t, "sw1-renamed", mod.Fields[0].NewValue)
}

func TestDiffSnapshots_TableFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows1 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "sb", Table: "Chassis", UUID: "uuid-2", Data: map[string]any{"hostname": "node1"}},
	}
	rows2 := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1-changed"}},
		{Database: "sb", Table: "Chassis", UUID: "uuid-2", Data: map[string]any{"hostname": "node1-changed"}},
	}

	snap1, err := store.CreateSnapshot(ctx, "manual", "", rows1)
	require.NoError(t, err)
	snap2, err := store.CreateSnapshot(ctx, "manual", "", rows2)
	require.NoError(t, err)

	diff, err := store.DiffSnapshots(ctx, snap1.ID, snap2.ID, "nb.Logical_Switch")
	require.NoError(t, err)
	require.Len(t, diff.Tables, 1)
	assert.Equal(t, "Logical_Switch", diff.Tables[0].Table)
}
