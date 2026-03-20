package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SnapshotMeta contains metadata about a snapshot.
type SnapshotMeta struct {
	ID        int64          `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Trigger   string         `json:"trigger"`
	Label     string         `json:"label"`
	RowCounts map[string]int `json:"row_counts"`
	SizeBytes int64          `json:"size_bytes"`
}

// SnapshotRow represents a single row within a snapshot.
type SnapshotRow struct {
	Database string         `json:"database"`
	Table    string         `json:"table"`
	UUID     string         `json:"uuid"`
	Data     map[string]any `json:"data"`
}

// CreateSnapshot stores a full snapshot in a single transaction.
func (s *Store) CreateSnapshot(ctx context.Context, trigger, label string, rows []SnapshotRow) (*SnapshotMeta, error) {
	now := time.Now().UTC()

	// Compute row counts per db.table
	rowCounts := make(map[string]int)
	for _, r := range rows {
		key := r.Database + "." + r.Table
		rowCounts[key]++
	}
	countsJSON, err := json.Marshal(rowCounts)
	if err != nil {
		return nil, fmt.Errorf("marshaling row counts: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		"INSERT INTO snapshots (timestamp, trigger, label, row_counts, size_bytes) VALUES (?, ?, ?, ?, 0)",
		now, trigger, label, string(countsJSON))
	if err != nil {
		return nil, fmt.Errorf("inserting snapshot: %w", err)
	}
	snapshotID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting snapshot id: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO snapshot_rows (snapshot_id, database, table_name, uuid, data) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return nil, fmt.Errorf("preparing row insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	var totalSize int64
	for _, r := range rows {
		dataJSON, err := json.Marshal(r.Data)
		if err != nil {
			return nil, fmt.Errorf("marshaling row data: %w", err)
		}
		compressed, err := compress(dataJSON)
		if err != nil {
			return nil, fmt.Errorf("compressing row data: %w", err)
		}
		totalSize += int64(len(compressed))

		if _, err := stmt.ExecContext(ctx, snapshotID, r.Database, r.Table, r.UUID, compressed); err != nil {
			return nil, fmt.Errorf("inserting row: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, "UPDATE snapshots SET size_bytes = ? WHERE id = ?", totalSize, snapshotID); err != nil {
		return nil, fmt.Errorf("updating size: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing snapshot: %w", err)
	}

	return &SnapshotMeta{
		ID:        snapshotID,
		Timestamp: now,
		Trigger:   trigger,
		Label:     label,
		RowCounts: rowCounts,
		SizeBytes: totalSize,
	}, nil
}

// ListSnapshots returns all snapshot metadata, newest first.
func (s *Store) ListSnapshots(ctx context.Context) ([]SnapshotMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, timestamp, trigger, label, row_counts, size_bytes FROM snapshots ORDER BY timestamp DESC")
	if err != nil {
		return nil, fmt.Errorf("querying snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []SnapshotMeta
	for rows.Next() {
		var m SnapshotMeta
		var countsStr string
		if err := rows.Scan(&m.ID, &m.Timestamp, &m.Trigger, &m.Label, &countsStr, &m.SizeBytes); err != nil {
			return nil, fmt.Errorf("scanning snapshot: %w", err)
		}
		m.RowCounts = make(map[string]int)
		if err := json.Unmarshal([]byte(countsStr), &m.RowCounts); err != nil {
			return nil, fmt.Errorf("parsing row counts: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// GetSnapshot returns metadata for a single snapshot.
func (s *Store) GetSnapshot(ctx context.Context, id int64) (*SnapshotMeta, error) {
	var m SnapshotMeta
	var countsStr string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, timestamp, trigger, label, row_counts, size_bytes FROM snapshots WHERE id = ?", id,
	).Scan(&m.ID, &m.Timestamp, &m.Trigger, &m.Label, &countsStr, &m.SizeBytes)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("snapshot %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying snapshot %d: %w", id, err)
	}
	m.RowCounts = make(map[string]int)
	if err := json.Unmarshal([]byte(countsStr), &m.RowCounts); err != nil {
		return nil, fmt.Errorf("parsing row counts: %w", err)
	}
	return &m, nil
}

// GetSnapshotRows returns rows from a snapshot, optionally filtered by database and table.
func (s *Store) GetSnapshotRows(ctx context.Context, id int64, database, table string) ([]SnapshotRow, error) {
	query := "SELECT database, table_name, uuid, data FROM snapshot_rows WHERE snapshot_id = ?"
	args := []any{id}
	if database != "" {
		query += " AND database = ?"
		args = append(args, database)
	}
	if table != "" {
		query += " AND table_name = ?"
		args = append(args, table)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying snapshot rows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []SnapshotRow
	for rows.Next() {
		var sr SnapshotRow
		var compressed []byte
		if err := rows.Scan(&sr.Database, &sr.Table, &sr.UUID, &compressed); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		decompressed, err := decompress(compressed)
		if err != nil {
			return nil, fmt.Errorf("decompressing row: %w", err)
		}
		sr.Data = make(map[string]any)
		if err := json.Unmarshal(decompressed, &sr.Data); err != nil {
			return nil, fmt.Errorf("unmarshaling row data: %w", err)
		}
		result = append(result, sr)
	}
	return result, rows.Err()
}

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = fmt.Errorf("not found")

// DeleteSnapshot removes a snapshot and its rows (via CASCADE).
func (s *Store) DeleteSnapshot(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM snapshots WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting snapshot %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("snapshot %d: %w", id, ErrNotFound)
	}
	return nil
}
