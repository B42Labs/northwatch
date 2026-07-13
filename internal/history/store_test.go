package history

import (
	"context"
	"database/sql"
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

// TestNewStore_PragmasApplyToEveryConnection is the regression test for a bug
// that silently corrupted data: SQLite pragmas are per-connection, and
// database/sql keeps an unbounded pool, so setting them once after Open left
// every other pooled connection with foreign_keys=OFF.
func TestNewStore_PragmasApplyToEveryConnection(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "pragma.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	db := store.DB()
	db.SetMaxOpenConns(4)

	// Hold several connections open at once so the pool must hand out distinct
	// ones, then check each individually.
	var conns []*sql.Conn
	for range 4 {
		c, err := db.Conn(ctx)
		require.NoError(t, err)
		conns = append(conns, c)
	}
	for i, c := range conns {
		var foreignKeys int
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys))
		assert.Equal(t, 1, foreignKeys, "connection %d must enforce foreign keys", i)

		var busyTimeout int
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout))
		assert.Equal(t, 5000, busyTimeout, "connection %d must have a busy timeout", i)

		require.NoError(t, c.Close())
	}
}
