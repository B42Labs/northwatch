package history

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// EventRecord represents a persisted OVSDB change event.
type EventRecord struct {
	ID        int64          `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"`
	Database  string         `json:"database"`
	Table     string         `json:"table"`
	UUID      string         `json:"uuid"`
	Row       map[string]any `json:"row,omitempty"`
	OldRow    map[string]any `json:"old_row,omitempty"`
}

// EventQueryOpts controls event log queries.
type EventQueryOpts struct {
	Since    *time.Time
	Until    *time.Time
	Database string
	Table    string
	Type     string
	Limit    int // default 1000, max 10000
}

// InsertEvent persists a single event.
func (s *Store) InsertEvent(ctx context.Context, e EventRecord) error {
	return s.InsertEvents(ctx, []EventRecord{e})
}

// InsertEvents persists a batch of events in a single transaction.
func (s *Store) InsertEvents(ctx context.Context, events []EventRecord) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO events (timestamp, type, database, table_name, uuid, row, old_row) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, e := range events {
		var rowBlob, oldRowBlob []byte

		if e.Row != nil {
			j, err := json.Marshal(e.Row)
			if err != nil {
				return fmt.Errorf("marshaling row: %w", err)
			}
			rowBlob, err = compress(j)
			if err != nil {
				return fmt.Errorf("compressing row: %w", err)
			}
		}
		if e.OldRow != nil {
			j, err := json.Marshal(e.OldRow)
			if err != nil {
				return fmt.Errorf("marshaling old_row: %w", err)
			}
			oldRowBlob, err = compress(j)
			if err != nil {
				return fmt.Errorf("compressing old_row: %w", err)
			}
		}

		ts := e.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		if _, err := stmt.ExecContext(ctx, ts, e.Type, e.Database, e.Table, e.UUID, rowBlob, oldRowBlob); err != nil {
			return fmt.Errorf("inserting event: %w", err)
		}
	}

	return tx.Commit()
}

// QueryEvents retrieves events matching the given filters.
func (s *Store) QueryEvents(ctx context.Context, opts EventQueryOpts) ([]EventRecord, error) {
	query := "SELECT id, timestamp, type, database, table_name, uuid, row, old_row FROM events WHERE 1=1"
	var args []any

	if opts.Since != nil {
		query += " AND timestamp >= ?"
		args = append(args, *opts.Since)
	}
	if opts.Until != nil {
		query += " AND timestamp <= ?"
		args = append(args, *opts.Until)
	}
	if opts.Database != "" {
		query += " AND database = ?"
		args = append(args, opts.Database)
	}
	if opts.Table != "" {
		query += " AND table_name = ?"
		args = append(args, opts.Table)
	}
	if opts.Type != "" {
		query += " AND type = ?"
		args = append(args, opts.Type)
	}

	query += " ORDER BY timestamp DESC"

	limit := opts.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}
	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []EventRecord
	for rows.Next() {
		var e EventRecord
		var rowBlob, oldRowBlob []byte
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Type, &e.Database, &e.Table, &e.UUID, &rowBlob, &oldRowBlob); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		if rowBlob != nil {
			decompressed, err := decompress(rowBlob)
			if err != nil {
				return nil, fmt.Errorf("decompressing row: %w", err)
			}
			e.Row = make(map[string]any)
			if err := json.Unmarshal(decompressed, &e.Row); err != nil {
				return nil, fmt.Errorf("unmarshaling row: %w", err)
			}
		}
		if oldRowBlob != nil {
			decompressed, err := decompress(oldRowBlob)
			if err != nil {
				return nil, fmt.Errorf("decompressing old_row: %w", err)
			}
			e.OldRow = make(map[string]any)
			if err := json.Unmarshal(decompressed, &e.OldRow); err != nil {
				return nil, fmt.Errorf("unmarshaling old_row: %w", err)
			}
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// PruneEvents deletes events older than the given retention duration.
func (s *Store) PruneEvents(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-retention)
	res, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning events: %w", err)
	}
	return res.RowsAffected()
}

// PruneEventsByCount keeps only the most recent maxCount events,
// deleting any older surplus. Returns the number of deleted events.
// If maxCount is 0, no pruning is performed.
func (s *Store) PruneEventsByCount(ctx context.Context, maxCount int64) (int64, error) {
	if maxCount <= 0 {
		return 0, nil
	}

	res, err := s.db.ExecContext(ctx,
		"DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT ?)",
		maxCount)
	if err != nil {
		return 0, fmt.Errorf("pruning events by count: %w", err)
	}
	return res.RowsAffected()
}

// EventCount returns the total number of persisted events.
func (s *Store) EventCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting events: %w", err)
	}
	return count, nil
}
