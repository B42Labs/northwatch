package history

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportSnapshot(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "sb", Table: "Chassis", UUID: "uuid-2", Data: map[string]any{"hostname": "node1"}},
	}

	meta, err := store.CreateSnapshot(ctx, "manual", "test export", rows)
	require.NoError(t, err)

	export, err := store.ExportSnapshot(ctx, meta.ID)
	require.NoError(t, err)

	assert.Equal(t, meta.ID, export.Meta.ID)
	assert.Equal(t, "manual", export.Meta.Trigger)
	assert.Equal(t, "test export", export.Meta.Label)
	assert.Len(t, export.Rows, 2)

	// Verify row data round-tripped correctly
	foundSwitch := false
	foundChassis := false
	for _, r := range export.Rows {
		if r.Database == "nb" && r.Table == "Logical_Switch" {
			assert.Equal(t, "sw1", r.Data["name"])
			foundSwitch = true
		}
		if r.Database == "sb" && r.Table == "Chassis" {
			assert.Equal(t, "node1", r.Data["hostname"])
			foundChassis = true
		}
	}
	assert.True(t, foundSwitch, "should have NB switch row")
	assert.True(t, foundChassis, "should have SB chassis row")
}

func TestExportSnapshot_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.ExportSnapshot(ctx, 999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestImportSnapshot(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create and export
	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "nb", Table: "ACL", UUID: "uuid-2", Data: map[string]any{"action": "allow"}},
	}
	original, err := store.CreateSnapshot(ctx, "manual", "original", rows)
	require.NoError(t, err)

	export, err := store.ExportSnapshot(ctx, original.ID)
	require.NoError(t, err)

	// Import into same store (simulates restore)
	imported, err := store.ImportSnapshot(ctx, *export)
	require.NoError(t, err)

	assert.NotEqual(t, original.ID, imported.ID, "imported snapshot should get a new ID")
	assert.Equal(t, "manual", imported.Trigger)
	assert.Equal(t, "original", imported.Label)
	assert.Equal(t, 1, imported.RowCounts["nb.Logical_Switch"])
	assert.Equal(t, 1, imported.RowCounts["nb.ACL"])

	// Verify imported rows
	importedRows, err := store.GetSnapshotRows(ctx, imported.ID, "", "")
	require.NoError(t, err)
	assert.Len(t, importedRows, 2)
}

func TestImportSnapshot_CrossStore(t *testing.T) {
	storeA := newTestStore(t)
	storeB := newTestStore(t)
	ctx := context.Background()

	// Create snapshot in store A
	rows := []SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	meta, err := storeA.CreateSnapshot(ctx, "auto", "periodic", rows)
	require.NoError(t, err)

	// Export from store A
	export, err := storeA.ExportSnapshot(ctx, meta.ID)
	require.NoError(t, err)

	// Import into store B
	imported, err := storeB.ImportSnapshot(ctx, *export)
	require.NoError(t, err)

	assert.Equal(t, "auto", imported.Trigger)
	assert.Equal(t, "periodic", imported.Label)

	// Verify data in store B
	importedRows, err := storeB.GetSnapshotRows(ctx, imported.ID, "", "")
	require.NoError(t, err)
	require.Len(t, importedRows, 1)
	assert.Equal(t, "sw1", importedRows[0].Data["name"])
}
