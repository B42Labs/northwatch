package write

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// maxDecompressedSize limits the decompressed output to 10 MB to prevent decompression bombs.
const maxDecompressedSize = 10 << 20

// AuditStore persists write audit entries in SQLite.
type AuditStore struct {
	db *sql.DB
}

// NewAuditStore creates a new AuditStore, creating the table if it does not exist.
func NewAuditStore(ctx context.Context, db *sql.DB) (*AuditStore, error) {
	stmt := `CREATE TABLE IF NOT EXISTS write_audit (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp   DATETIME NOT NULL,
		plan_id     TEXT NOT NULL,
		actor       TEXT NOT NULL DEFAULT '',
		reason      TEXT NOT NULL DEFAULT '',
		operations  BLOB NOT NULL,
		snapshot_id INTEGER NOT NULL DEFAULT 0,
		result      TEXT NOT NULL DEFAULT '',
		error_msg   TEXT NOT NULL DEFAULT ''
	)`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return nil, fmt.Errorf("creating write_audit table: %w", err)
	}
	return &AuditStore{db: db}, nil
}

// Insert records an audit entry.
func (s *AuditStore) Insert(ctx context.Context, entry AuditEntry) error {
	opsJSON, err := json.Marshal(entry.Operations)
	if err != nil {
		return fmt.Errorf("marshalling operations: %w", err)
	}

	compressed, err := compressData(opsJSON)
	if err != nil {
		return fmt.Errorf("compressing operations: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO write_audit (timestamp, plan_id, actor, reason, operations, snapshot_id, result, error_msg)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Timestamp.UTC(), entry.PlanID, entry.Actor, entry.Reason,
		compressed, entry.SnapshotID, entry.Result, entry.Error,
	)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}
	return nil
}

// Query returns the most recent audit entries up to the given limit.
func (s *AuditStore) Query(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, timestamp, plan_id, actor, reason, operations, snapshot_id, result, error_msg
		 FROM write_audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying audit entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []AuditEntry
	for rows.Next() {
		entry, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// GetByID returns a single audit entry by its ID.
func (s *AuditStore) GetByID(ctx context.Context, id int64) (*AuditEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, timestamp, plan_id, actor, reason, operations, snapshot_id, result, error_msg
		 FROM write_audit WHERE id = ?`, id)

	var entry AuditEntry
	var ts time.Time
	var opsBlob []byte
	err := row.Scan(&entry.ID, &ts, &entry.PlanID, &entry.Actor, &entry.Reason,
		&opsBlob, &entry.SnapshotID, &entry.Result, &entry.Error)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("audit entry %d not found", id)
		}
		return nil, fmt.Errorf("scanning audit entry: %w", err)
	}
	entry.Timestamp = ts.UTC()

	decompressed, err := decompressData(opsBlob)
	if err != nil {
		return nil, fmt.Errorf("decompressing operations: %w", err)
	}
	if err := json.Unmarshal(decompressed, &entry.Operations); err != nil {
		return nil, fmt.Errorf("unmarshalling operations: %w", err)
	}

	return &entry, nil
}

// scanAuditRow scans a single row from a query result.
func scanAuditRow(rows *sql.Rows) (AuditEntry, error) {
	var entry AuditEntry
	var ts time.Time
	var opsBlob []byte
	err := rows.Scan(&entry.ID, &ts, &entry.PlanID, &entry.Actor, &entry.Reason,
		&opsBlob, &entry.SnapshotID, &entry.Result, &entry.Error)
	if err != nil {
		return AuditEntry{}, fmt.Errorf("scanning audit row: %w", err)
	}
	entry.Timestamp = ts.UTC()

	decompressed, err := decompressData(opsBlob)
	if err != nil {
		return AuditEntry{}, fmt.Errorf("decompressing operations: %w", err)
	}
	if err := json.Unmarshal(decompressed, &entry.Operations); err != nil {
		return AuditEntry{}, fmt.Errorf("unmarshalling operations: %w", err)
	}

	return entry, nil
}

func compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressData(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(io.LimitReader(r, maxDecompressedSize))
}
