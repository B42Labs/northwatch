package write

import "time"

// WriteOperation describes a single intended mutation.
type WriteOperation struct {
	Action string         `json:"action"`           // "create", "update", "delete"
	Table  string         `json:"table"`            // OVN NB table name
	UUID   string         `json:"uuid,omitempty"`   // required for update/delete
	Fields map[string]any `json:"fields,omitempty"` // desired field values
	Reason string         `json:"reason,omitempty"`
}

// Plan is the preview/dry-run result.
type Plan struct {
	ID         string           `json:"id"`
	CreatedAt  time.Time        `json:"created_at"`
	ExpiresAt  time.Time        `json:"expires_at"`
	Operations []WriteOperation `json:"operations"`
	Diffs      []PlanDiff       `json:"diffs"`
	SnapshotID int64            `json:"snapshot_id"`
	Status     string           `json:"status"` // "pending", "applied", "expired", "failed", "dry-run"
	ApplyToken string           `json:"apply_token"`
}

// PlanDiff shows what one operation will change.
type PlanDiff struct {
	Action string         `json:"action"`
	Table  string         `json:"table"`
	UUID   string         `json:"uuid,omitempty"`
	Before map[string]any `json:"before,omitempty"`
	After  map[string]any `json:"after,omitempty"`
	Fields []FieldChange  `json:"fields,omitempty"`
}

// FieldChange represents a single field difference.
type FieldChange struct {
	Field    string `json:"field"`
	OldValue any    `json:"old_value,omitempty"`
	NewValue any    `json:"new_value,omitempty"`
}

// AuditEntry records a write operation.
type AuditEntry struct {
	ID         int64            `json:"id"`
	Timestamp  time.Time        `json:"timestamp"`
	PlanID     string           `json:"plan_id"`
	Actor      string           `json:"actor"`
	Reason     string           `json:"reason"`
	Operations []WriteOperation `json:"operations"`
	SnapshotID int64            `json:"snapshot_id"`
	Result     string           `json:"result"` // "success" or "error"
	Error      string           `json:"error,omitempty"`
}
