package history

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// DiffResult describes all differences between two snapshots.
type DiffResult struct {
	FromID int64       `json:"from_id"`
	ToID   int64       `json:"to_id"`
	Tables []TableDiff `json:"tables"`
}

// TableDiff describes changes within a single database table.
type TableDiff struct {
	Database string           `json:"database"`
	Table    string           `json:"table"`
	Added    []map[string]any `json:"added"`
	Removed  []map[string]any `json:"removed"`
	Modified []RowDiff        `json:"modified"`
}

// RowDiff describes field-level changes to a single row.
type RowDiff struct {
	UUID   string        `json:"uuid"`
	Fields []FieldChange `json:"fields"`
}

// FieldChange describes a single field's old and new values.
type FieldChange struct {
	Field    string `json:"field"`
	OldValue any    `json:"old_value"`
	NewValue any    `json:"new_value"`
}

// DiffSnapshots computes field-level differences between two snapshots.
// An optional tableFilter restricts comparison to a specific "database.table".
func (s *Store) DiffSnapshots(ctx context.Context, fromID, toID int64, tableFilter string) (*DiffResult, error) {
	// When a table filter is provided, only load rows for that table
	var filterDB, filterTable string
	if tableFilter != "" {
		if idx := strings.Index(tableFilter, "."); idx > 0 {
			filterDB = tableFilter[:idx]
			filterTable = tableFilter[idx+1:]
		}
	}

	fromRows, err := s.GetSnapshotRows(ctx, fromID, filterDB, filterTable)
	if err != nil {
		return nil, fmt.Errorf("loading from-snapshot rows: %w", err)
	}
	toRows, err := s.GetSnapshotRows(ctx, toID, filterDB, filterTable)
	if err != nil {
		return nil, fmt.Errorf("loading to-snapshot rows: %w", err)
	}

	// Build maps keyed by "db.table" -> uuid -> data
	type rowKey struct {
		Database string
		Table    string
	}
	fromMap := make(map[rowKey]map[string]map[string]any)
	toMap := make(map[rowKey]map[string]map[string]any)
	allTables := make(map[rowKey]bool)

	for _, r := range fromRows {
		k := rowKey{r.Database, r.Table}
		allTables[k] = true
		if fromMap[k] == nil {
			fromMap[k] = make(map[string]map[string]any)
		}
		fromMap[k][r.UUID] = r.Data
	}
	for _, r := range toRows {
		k := rowKey{r.Database, r.Table}
		allTables[k] = true
		if toMap[k] == nil {
			toMap[k] = make(map[string]map[string]any)
		}
		toMap[k][r.UUID] = r.Data
	}

	// Sort table keys for deterministic output
	var tableKeys []rowKey
	for k := range allTables {
		tableKeys = append(tableKeys, k)
	}
	sort.Slice(tableKeys, func(i, j int) bool {
		if tableKeys[i].Database != tableKeys[j].Database {
			return tableKeys[i].Database < tableKeys[j].Database
		}
		return tableKeys[i].Table < tableKeys[j].Table
	})

	result := &DiffResult{FromID: fromID, ToID: toID}

	for _, k := range tableKeys {
		fromUUIDs := fromMap[k]
		toUUIDs := toMap[k]
		if fromUUIDs == nil {
			fromUUIDs = make(map[string]map[string]any)
		}
		if toUUIDs == nil {
			toUUIDs = make(map[string]map[string]any)
		}

		td := TableDiff{Database: k.Database, Table: k.Table}

		// Find added and modified
		for uuid, toData := range toUUIDs {
			fromData, existed := fromUUIDs[uuid]
			if !existed {
				td.Added = append(td.Added, toData)
				continue
			}
			// Check for modifications
			changes := diffFields(fromData, toData)
			if len(changes) > 0 {
				td.Modified = append(td.Modified, RowDiff{UUID: uuid, Fields: changes})
			}
		}

		// Find removed
		for uuid, fromData := range fromUUIDs {
			if _, exists := toUUIDs[uuid]; !exists {
				td.Removed = append(td.Removed, fromData)
			}
		}

		if len(td.Added) > 0 || len(td.Removed) > 0 || len(td.Modified) > 0 {
			result.Tables = append(result.Tables, td)
		}
	}

	return result, nil
}

func diffFields(old, new map[string]any) []FieldChange {
	var changes []FieldChange

	// Collect all keys
	keys := make(map[string]bool)
	for k := range old {
		keys[k] = true
	}
	for k := range new {
		keys[k] = true
	}

	// Sort for deterministic output
	var sortedKeys []string
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		oldVal, oldOk := old[k]
		newVal, newOk := new[k]
		if !oldOk {
			changes = append(changes, FieldChange{Field: k, OldValue: nil, NewValue: newVal})
		} else if !newOk {
			changes = append(changes, FieldChange{Field: k, OldValue: oldVal, NewValue: nil})
		} else if !reflect.DeepEqual(oldVal, newVal) {
			changes = append(changes, FieldChange{Field: k, OldValue: oldVal, NewValue: newVal})
		}
	}
	return changes
}
