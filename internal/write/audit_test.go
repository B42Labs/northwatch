package write

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func setupTestAuditStore(t *testing.T) *AuditStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store, err := NewAuditStore(db)
	require.NoError(t, err)
	return store
}

func TestAuditStore_InsertAndQuery(t *testing.T) {
	store := setupTestAuditStore(t)
	ctx := context.Background()

	entry := AuditEntry{
		Timestamp: time.Now().UTC(),
		PlanID:    "plan-1",
		Actor:     "admin",
		Reason:    "test operation",
		Operations: []WriteOperation{
			{
				Action: "create",
				Table:  "Logical_Switch",
				Fields: map[string]any{"name": "test-switch"},
			},
		},
		SnapshotID: 42,
		Result:     "success",
	}
	err := store.Insert(ctx, entry)
	require.NoError(t, err)

	entries, err := store.Query(ctx, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	got := entries[0]
	assert.Equal(t, "plan-1", got.PlanID)
	assert.Equal(t, "admin", got.Actor)
	assert.Equal(t, "test operation", got.Reason)
	assert.Equal(t, int64(42), got.SnapshotID)
	assert.Equal(t, "success", got.Result)
	require.Len(t, got.Operations, 1)
	assert.Equal(t, "create", got.Operations[0].Action)
	assert.Equal(t, "Logical_Switch", got.Operations[0].Table)
}

func TestAuditStore_GetByID(t *testing.T) {
	store := setupTestAuditStore(t)
	ctx := context.Background()

	entry := AuditEntry{
		Timestamp: time.Now().UTC(),
		PlanID:    "plan-2",
		Actor:     "user1",
		Reason:    "delete port",
		Operations: []WriteOperation{
			{Action: "delete", Table: "Logical_Switch_Port", UUID: "uuid-123"},
		},
		SnapshotID: 10,
		Result:     "success",
	}
	require.NoError(t, store.Insert(ctx, entry))

	got, err := store.GetByID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "plan-2", got.PlanID)
	assert.Equal(t, "user1", got.Actor)
	assert.Equal(t, int64(1), got.ID)
}

func TestAuditStore_GetByID_NotFound(t *testing.T) {
	store := setupTestAuditStore(t)
	ctx := context.Background()

	_, err := store.GetByID(ctx, 999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestAuditStore_QueryLimit(t *testing.T) {
	store := setupTestAuditStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		err := store.Insert(ctx, AuditEntry{
			Timestamp:  time.Now().UTC(),
			PlanID:     "plan",
			Operations: []WriteOperation{{Action: "create", Table: "ACL"}},
			Result:     "success",
		})
		require.NoError(t, err)
	}

	entries, err := store.Query(ctx, 3)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestAuditStore_ErrorField(t *testing.T) {
	store := setupTestAuditStore(t)
	ctx := context.Background()

	entry := AuditEntry{
		Timestamp:  time.Now().UTC(),
		PlanID:     "plan-fail",
		Operations: []WriteOperation{{Action: "update", Table: "ACL", UUID: "u1"}},
		Result:     "error",
		Error:      "transaction failed: constraint violation",
	}
	require.NoError(t, store.Insert(ctx, entry))

	got, err := store.GetByID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "error", got.Result)
	assert.Equal(t, "transaction failed: constraint violation", got.Error)
}
