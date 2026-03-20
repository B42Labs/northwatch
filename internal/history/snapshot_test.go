package history

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAndListSnapshots(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-2", Data: map[string]any{"name": "sw2"}},
		{Database: "sb", Table: "Chassis", UUID: "uuid-3", Data: map[string]any{"hostname": "node1"}},
	}

	meta, err := store.CreateSnapshot(ctx, "manual", "test snapshot", rows)
	require.NoError(t, err)
	assert.Equal(t, int64(1), meta.ID)
	assert.Equal(t, "manual", meta.Trigger)
	assert.Equal(t, "test snapshot", meta.Label)
	assert.Equal(t, 2, meta.RowCounts["nb.Logical_Switch"])
	assert.Equal(t, 1, meta.RowCounts["sb.Chassis"])
	assert.Greater(t, meta.SizeBytes, int64(0))

	list, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, meta.ID, list[0].ID)
}

func TestGetSnapshot(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	created, err := store.CreateSnapshot(ctx, "auto", "periodic", rows)
	require.NoError(t, err)

	got, err := store.GetSnapshot(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "auto", got.Trigger)
	assert.Equal(t, "periodic", got.Label)
}

func TestGetSnapshotRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "nb", Table: "ACL", UUID: "uuid-2", Data: map[string]any{"action": "allow"}},
		{Database: "sb", Table: "Chassis", UUID: "uuid-3", Data: map[string]any{"hostname": "node1"}},
	}
	meta, err := store.CreateSnapshot(ctx, "manual", "", rows)
	require.NoError(t, err)

	t.Run("all rows", func(t *testing.T) {
		got, err := store.GetSnapshotRows(ctx, meta.ID, "", "")
		require.NoError(t, err)
		assert.Len(t, got, 3)
	})

	t.Run("filter by database", func(t *testing.T) {
		got, err := store.GetSnapshotRows(ctx, meta.ID, "nb", "")
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("filter by database and table", func(t *testing.T) {
		got, err := store.GetSnapshotRows(ctx, meta.ID, "nb", "Logical_Switch")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "sw1", got[0].Data["name"])
	})
}

func TestDeleteSnapshot_Cascade(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	meta, err := store.CreateSnapshot(ctx, "manual", "", rows)
	require.NoError(t, err)

	err = store.DeleteSnapshot(ctx, meta.ID)
	require.NoError(t, err)

	// Should be gone
	list, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)

	// Rows should be cascaded
	got, err := store.GetSnapshotRows(ctx, meta.ID, "", "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestDeleteSnapshot_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.DeleteSnapshot(ctx, 999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestGetSnapshot_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.GetSnapshot(ctx, 999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestCreateSnapshot_EmptyRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	meta, err := store.CreateSnapshot(ctx, "auto", "", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), meta.SizeBytes)
	assert.Empty(t, meta.RowCounts)
}
