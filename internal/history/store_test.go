package history

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestNewStore_WALMode(t *testing.T) {
	store := newTestStore(t)
	var journalMode string
	err := store.db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode)
}

func TestNewStore_ForeignKeys(t *testing.T) {
	store := newTestStore(t)
	var fk int
	err := store.db.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&fk)
	require.NoError(t, err)
	assert.Equal(t, 1, fk)
}

func TestNewStore_BusyTimeout(t *testing.T) {
	store := newTestStore(t)
	var timeout int
	err := store.db.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout)
	require.NoError(t, err)
	assert.Equal(t, 5000, timeout)
}

func TestMigration_Idempotent(t *testing.T) {
	store := newTestStore(t)
	// Running migrate again should not fail
	require.NoError(t, store.migrate())
	require.NoError(t, store.migrate())
}

func TestNewStore_TablesExist(t *testing.T) {
	store := newTestStore(t)

	tables := []string{"snapshots", "snapshot_rows", "events"}
	for _, table := range tables {
		var name string
		err := store.db.QueryRowContext(context.Background(),
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		require.NoError(t, err, "table %s should exist", table)
		assert.Equal(t, table, name)
	}
}
