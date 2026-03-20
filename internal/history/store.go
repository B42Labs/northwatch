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
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	pragmas := []struct{ name, stmt string }{
		{"WAL mode", "PRAGMA journal_mode=WAL"},
		{"foreign keys", "PRAGMA foreign_keys=ON"},
		{"busy timeout", "PRAGMA busy_timeout=5000"},
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(context.Background(), p.stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("setting %s: %w", p.name, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS snapshots (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp  DATETIME NOT NULL,
			trigger    TEXT NOT NULL,
			label      TEXT DEFAULT '',
			row_counts TEXT DEFAULT '{}',
			size_bytes INTEGER DEFAULT 0
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

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt[:40], err)
		}
	}
	return nil
}
