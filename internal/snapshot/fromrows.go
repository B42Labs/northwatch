package snapshot

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

// RowInput is one row to assemble into a snapshot, in the shape Northwatch's
// history store keeps: the column values keyed by their OVSDB tag (e.g.
// "_uuid", "external_ids"), which is what api.ModelToMap emits.
type RowInput struct {
	Database string // "nb" or "sb"
	Table    string
	UUID     string
	Data     map[string]any
}

// BuildFromRows assembles a snapshot File from history-style rows so a stored
// history snapshot can be replayed through Serve, exactly like a captured file.
//
// Each row's Data is keyed by OVSDB column tag, but Record.Model must JSON-decode
// into the generated model by its Go field names. The two differ for multi-word
// columns ("external_ids" vs "ExternalIDs"), so the keys are remapped per table
// (tag → field name) before re-encoding. Rows whose Database is neither "nb" nor
// "sb", or whose Table is unknown to the schema, are skipped.
func BuildFromRows(nbClientModel, sbClientModel model.ClientDBModel, nbSchema, sbSchema ovsdb.DatabaseSchema, rows []RowInput) (*File, error) {
	nbDBModel, errs := model.NewDatabaseModel(nbSchema, nbClientModel)
	if len(errs) > 0 {
		return nil, fmt.Errorf("building %s database model: %v", nbSchema.Name, errs)
	}
	sbDBModel, errs := model.NewDatabaseModel(sbSchema, sbClientModel)
	if len(errs) > 0 {
		return nil, fmt.Errorf("building %s database model: %v", sbSchema.Name, errs)
	}

	f := &File{
		Version: Version,
		NB:      Dump{Schema: nbSchema.Name, Tables: map[string][]Record{}},
		SB:      Dump{Schema: sbSchema.Name, Tables: map[string][]Record{}},
	}

	// A history snapshot captures only a curated subset of tables, so a captured
	// row may reference a row in a table that was not captured (e.g. Logical_Flow
	// → Logical_DP_Group). The in-memory server enforces referential integrity
	// and would reject the load, so collect every present UUID per database up
	// front and prune references to anything missing.
	nbPresent := map[string]struct{}{}
	sbPresent := map[string]struct{}{}
	for _, r := range rows {
		switch r.Database {
		case "nb":
			nbPresent[r.UUID] = struct{}{}
		case "sb":
			sbPresent[r.UUID] = struct{}{}
		}
	}

	// Field maps are built once per table and reused across that table's rows.
	nbFieldMaps := map[string]map[string]string{}
	sbFieldMaps := map[string]map[string]string{}

	for _, r := range rows {
		var dump *Dump
		var dbModel model.DatabaseModel
		var cache map[string]map[string]string
		var schema ovsdb.DatabaseSchema
		var present map[string]struct{}
		switch r.Database {
		case "nb":
			dump, dbModel, cache, schema, present = &f.NB, nbDBModel, nbFieldMaps, nbSchema, nbPresent
		case "sb":
			dump, dbModel, cache, schema, present = &f.SB, sbDBModel, sbFieldMaps, sbSchema, sbPresent
		default:
			continue
		}

		fieldMap, ok := cache[r.Table]
		if !ok {
			var err error
			fieldMap, err = tagToFieldMap(dbModel, r.Table)
			if err != nil {
				// Unknown table for this schema — skip rather than fail the load.
				cache[r.Table] = nil
				continue
			}
			cache[r.Table] = fieldMap
		}
		if fieldMap == nil {
			continue
		}

		data := pruneDanglingRefs(schema, r.Table, r.Data, present)
		remapped := make(map[string]any, len(data))
		for tag, v := range data {
			if field, ok := fieldMap[tag]; ok {
				remapped[field] = v
			}
		}
		raw, err := json.Marshal(remapped)
		if err != nil {
			return nil, fmt.Errorf("%s/%s %s: marshaling row: %w", r.Database, r.Table, r.UUID, err)
		}
		dump.Tables[r.Table] = append(dump.Tables[r.Table], Record{UUID: r.UUID, Model: raw})
	}

	return f, nil
}

// pruneDanglingRefs returns a copy of data with any UUID reference that points
// to a row not in present removed, so the in-memory server's referential
// integrity check passes. Columns are keyed by OVSDB tag, matching the schema.
// Non-reference columns and references to present rows are left untouched; the
// original map is returned unchanged when nothing needs pruning.
func pruneDanglingRefs(schema ovsdb.DatabaseSchema, table string, data map[string]any, present map[string]struct{}) map[string]any {
	ts, ok := schema.Tables[table]
	if !ok {
		return data
	}

	out := data
	copied := false
	ensureCopy := func() {
		if copied {
			return
		}
		out = make(map[string]any, len(data))
		for k, v := range data {
			out[k] = v
		}
		copied = true
	}
	isPresent := func(uuid string) bool {
		_, ok := present[uuid]
		return ok
	}

	for col, val := range data {
		cs := ts.Column(col)
		if cs == nil || cs.TypeObj == nil {
			continue
		}
		keyRef := isUUIDRef(cs.TypeObj.Key)
		valRef := isUUIDRef(cs.TypeObj.Value)
		if !keyRef && !valRef {
			continue
		}

		switch v := val.(type) {
		case string: // atomic reference (e.g. *string field)
			if keyRef && !isPresent(v) {
				ensureCopy()
				delete(out, col)
			}
		case []any: // set of references
			if !keyRef {
				continue
			}
			kept := make([]any, 0, len(v))
			for _, e := range v {
				if s, ok := e.(string); ok && !isPresent(s) {
					continue
				}
				kept = append(kept, e)
			}
			if len(kept) != len(v) {
				ensureCopy()
				out[col] = kept
			}
		case map[string]any: // map with references as keys and/or values
			changed := false
			kept := make(map[string]any, len(v))
			for mk, mv := range v {
				if keyRef && !isPresent(mk) {
					changed = true
					continue
				}
				if valRef {
					if s, ok := mv.(string); ok && !isPresent(s) {
						changed = true
						continue
					}
				}
				kept[mk] = mv
			}
			if changed {
				ensureCopy()
				out[col] = kept
			}
		}
	}
	return out
}

// pruneDanglingRowRefs is the wire-typed sibling of pruneDanglingRefs: it drops
// UUID references in a mapper-built ovsdb.Row that point to rows not in present,
// so the in-memory server's referential integrity check accepts a dump captured
// from a churning database. It operates on the OVSDB wire types (ovsdb.UUID,
// ovsdb.OvsSet, ovsdb.OvsMap) rather than the history-style native map.
func pruneDanglingRowRefs(schema ovsdb.DatabaseSchema, table string, row ovsdb.Row, present map[string]struct{}) ovsdb.Row {
	ts, ok := schema.Tables[table]
	if !ok {
		return row
	}
	isPresent := func(uuid string) bool {
		_, ok := present[uuid]
		return ok
	}

	for col, val := range row {
		cs := ts.Column(col)
		if cs == nil || cs.TypeObj == nil {
			continue
		}
		keyRef := isUUIDRef(cs.TypeObj.Key)
		valRef := isUUIDRef(cs.TypeObj.Value)
		if !keyRef && !valRef {
			continue
		}

		switch v := val.(type) {
		case ovsdb.UUID: // required atomic reference
			if keyRef && v.GoUUID != "" && !isPresent(v.GoUUID) {
				delete(row, col)
			}
		case ovsdb.OvsSet: // optional atomic (max 1) or set of references
			if !keyRef {
				continue
			}
			kept := make([]any, 0, len(v.GoSet))
			for _, e := range v.GoSet {
				if u, ok := e.(ovsdb.UUID); ok && !isPresent(u.GoUUID) {
					continue
				}
				kept = append(kept, e)
			}
			row[col] = ovsdb.OvsSet{GoSet: kept}
		case ovsdb.OvsMap: // map with references as keys and/or values
			kept := make(map[any]any, len(v.GoMap))
			for mk, mv := range v.GoMap {
				if keyRef {
					if u, ok := mk.(ovsdb.UUID); ok && !isPresent(u.GoUUID) {
						continue
					}
				}
				if valRef {
					if u, ok := mv.(ovsdb.UUID); ok && !isPresent(u.GoUUID) {
						continue
					}
				}
				kept[mk] = mv
			}
			row[col] = ovsdb.OvsMap{GoMap: kept}
		}
	}
	return row
}

// isUUIDRef reports whether a base type is a UUID that references another table.
func isUUIDRef(bt *ovsdb.BaseType) bool {
	if bt == nil || bt.Type != ovsdb.TypeUUID {
		return false
	}
	rt, err := bt.RefTable()
	return err == nil && rt != ""
}

// tagToFieldMap returns a mapping from each OVSDB column tag to the Go struct
// field name of the generated model for table. It mirrors api.ModelToMap's tag
// parsing so the round-trip is symmetric.
func tagToFieldMap(dbModel model.DatabaseModel, table string) (map[string]string, error) {
	m, err := dbModel.NewModel(table)
	if err != nil {
		return nil, err
	}
	t := reflect.TypeOf(m)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	out := make(map[string]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("ovsdb")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		out[tag] = field.Name
	}
	return out, nil
}
