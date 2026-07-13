package history

import (
	"context"
	"errors"
	"fmt"
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

// TestPruneAutoSnapshots covers snapshot retention. Auto-snapshots ran every 5
// minutes on change with no pruning at all — up to 288 full database copies a
// day, forever — while events were pruned all along.
func TestPruneAutoSnapshots(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	row := func(name string) []SnapshotRow {
		return []SnapshotRow{{Database: "nb", Table: "Logical_Switch", UUID: "uuid-" + name, Data: map[string]any{"name": name}}}
	}

	// Interleave the snapshots an operator deliberately kept with the automatic
	// ones, so pruning has to discriminate rather than just drop the oldest.
	var autoIDs []int64
	for i := range 5 {
		meta, err := store.CreateSnapshot(ctx, "auto", "", row(fmt.Sprintf("auto-%d", i)))
		require.NoError(t, err)
		autoIDs = append(autoIDs, meta.ID)
	}
	manual, err := store.CreateSnapshot(ctx, "manual", "", row("manual"))
	require.NoError(t, err)
	labeled, err := store.CreateSnapshot(ctx, "auto", "pre-upgrade", row("labeled"))
	require.NoError(t, err)

	n, err := store.PruneAutoSnapshots(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)

	list, err := store.ListSnapshots(ctx)
	require.NoError(t, err)

	kept := make(map[int64]bool, len(list))
	for _, m := range list {
		kept[m.ID] = true
	}

	// The three oldest autos are gone; the two newest survive.
	assert.False(t, kept[autoIDs[0]])
	assert.False(t, kept[autoIDs[1]])
	assert.False(t, kept[autoIDs[2]])
	assert.True(t, kept[autoIDs[3]])
	assert.True(t, kept[autoIDs[4]])

	// A manual snapshot and a labeled one are never reclaimed.
	assert.True(t, kept[manual.ID], "a manual snapshot must never be pruned")
	assert.True(t, kept[labeled.ID], "a labeled snapshot must never be pruned")

	// Pruning cascades to the rows of the snapshots it removed.
	var orphans int
	require.NoError(t, store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM snapshot_rows WHERE snapshot_id IN (?, ?, ?)",
		autoIDs[0], autoIDs[1], autoIDs[2]).Scan(&orphans))
	assert.Zero(t, orphans, "pruning must not orphan snapshot_rows")
}

func TestPruneAutoSnapshots_Disabled(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := store.CreateSnapshot(ctx, "auto", "",
			[]SnapshotRow{{Database: "nb", Table: "Logical_Switch", UUID: fmt.Sprintf("uuid-%d", i), Data: map[string]any{}}})
		require.NoError(t, err)
	}

	for _, keep := range []int64{0, -1} {
		n, err := store.PruneAutoSnapshots(ctx, keep)
		require.NoError(t, err)
		assert.Zero(t, n)
	}

	list, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestPruneAutoSnapshots_FewerThanLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.CreateSnapshot(ctx, "auto", "",
		[]SnapshotRow{{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{}}})
	require.NoError(t, err)

	n, err := store.PruneAutoSnapshots(ctx, 10)
	require.NoError(t, err)
	assert.Zero(t, n)

	list, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
