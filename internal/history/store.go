package history

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database for snapshot and event persistence.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database and runs migrations.
//
// The pragmas travel in the DSN rather than as one-shot statements after Open.
// SQLite pragmas are per-connection, and database/sql keeps an unbounded pool:
// executing them once applied them to whichever single connection happened to
// serve the Exec, so every other pooled connection ran with foreign_keys=OFF.
// DeleteSnapshot's ON DELETE CASCADE then silently orphaned snapshot_rows
// whenever it landed on one of those connections. Passing them via ?_pragma=
// makes modernc.org/sqlite apply them to every connection it opens.
func NewStore(dbPath string) (*Store, error) {
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return s, nil
}

// DB returns the underlying *sql.DB for shared use (e.g. audit store).
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	// Phase 1: create base tables and indexes
	baseStatements := []string{
		`CREATE TABLE IF NOT EXISTS snapshots (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp    DATETIME NOT NULL,
			trigger      TEXT NOT NULL,
			label        TEXT DEFAULT '',
			row_counts   TEXT DEFAULT '{}',
			size_bytes   INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS snapshot_rows (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
			database    TEXT NOT NULL,
			table_name  TEXT NOT NULL,
			uuid        TEXT NOT NULL,
			data        BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshot_rows_lookup
			ON snapshot_rows(snapshot_id, database, table_name)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshot_rows_unique
			ON snapshot_rows(snapshot_id, database, table_name, uuid)`,
		`CREATE TABLE IF NOT EXISTS events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp  DATETIME NOT NULL,
			type       TEXT NOT NULL,
			database   TEXT NOT NULL,
			table_name TEXT NOT NULL,
			uuid       TEXT NOT NULL,
			row        BLOB,
			old_row    BLOB
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_events_lookup ON events(database, table_name)`,
	}

	for _, stmt := range baseStatements {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt[:40], err)
		}
	}

	// Phase 2: incremental migrations (add columns to existing tables)
	if !s.hasColumn("snapshots", "content_hash") {
		if _, err := s.db.ExecContext(context.Background(),
			"ALTER TABLE snapshots ADD COLUMN content_hash TEXT DEFAULT ''"); err != nil {
			return fmt.Errorf("adding content_hash column: %w", err)
		}
	}

	return nil
}

// hasColumn checks whether a given table already has the named column.
func (s *Store) hasColumn(table, column string) bool {
	rows, err := s.db.QueryContext(context.Background(),
		fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
			if name == column {
				return true
			}
		}
	}
	return false
}
