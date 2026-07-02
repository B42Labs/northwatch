package write

import (
	"context"
	"fmt"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

// nbDatabaseSchema is the NB OVSDB schema, used to derive which columns hold
// UUID references so reference validation is not hand-maintained per table.
var nbDatabaseSchema = nb.Schema()

var validActions = map[string]bool{
	"create": true,
	"update": true,
	"delete": true,
}

// ValidateOperation checks that a WriteOperation is well-formed given the registry.
func ValidateOperation(op WriteOperation, reg *Registry) error {
	if !validActions[op.Action] {
		return fmt.Errorf("invalid action %q: must be create, update, or delete", op.Action)
	}

	spec, err := reg.Get(op.Table)
	if err != nil {
		return err
	}

	if spec.DeleteOnly && op.Action != "delete" {
		return fmt.Errorf("table %q only supports delete operations", op.Table)
	}

	switch op.Action {
	case "create":
		if op.UUID != "" {
			return fmt.Errorf("create operations must not specify a UUID")
		}
		if len(op.Fields) == 0 {
			return fmt.Errorf("create operations require at least one field")
		}
	case "update":
		if op.UUID == "" {
			return fmt.Errorf("update operations require a UUID")
		}
		if len(op.Fields) == 0 {
			return fmt.Errorf("update operations require at least one field")
		}
	case "delete":
		if op.UUID == "" {
			return fmt.Errorf("delete operations require a UUID")
		}
	}

	if len(op.Fields) > 0 {
		if err := ValidateFields(op.Fields, spec); err != nil {
			return err
		}
	}

	return nil
}

// ValidateSingleDatabase rejects plans that mix NB and SB operations,
// since they cannot be applied atomically across two OVSDB databases.
func ValidateSingleDatabase(ops []WriteOperation, reg *Registry) error {
	var hasNB, hasSB bool
	for _, op := range ops {
		spec, err := reg.Get(op.Table)
		if err != nil {
			return err
		}
		if spec.Database == "sb" {
			hasSB = true
		} else {
			hasNB = true
		}
		if hasNB && hasSB {
			return fmt.Errorf("plans cannot mix NB and SB operations; submit separate plans for each database")
		}
	}
	return nil
}

// ValidateFields checks that field names exist on the model and are not read-only.
func ValidateFields(fields map[string]any, spec TableSpec) error {
	validFields := make(map[string]bool)
	for _, name := range OVSDBFieldNames(spec.ModelType) {
		validFields[name] = true
	}

	var errs []string
	for name := range fields {
		if spec.ReadOnlyFields[name] {
			errs = append(errs, fmt.Sprintf("field %q is read-only", name))
			continue
		}
		if !validFields[name] {
			errs = append(errs, fmt.Sprintf("field %q does not exist on table %s", name, spec.Table))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("field validation errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ValidateReferences checks that referenced entities exist in the NB database.
// When cacheAuthoritative is true it first runs schema-derived UUID reference
// validation for every writable NB table, then applies the table-specific extra
// checks. User-facing failures (a missing reference, a duplicate, an
// out-of-range value) are returned as *InputError; genuine infrastructure
// failures (a cache list error) are returned unwrapped so handlers map them to
// 500 rather than 400.
//
// Every check that consults the local cache — the schema-derived UUID check and
// the cache-backed table-specific checks (Logical_Switch_Port router-port
// existence, NAT duplicate detection) — is gated on cacheAuthoritative. The
// cache is only authoritative while the NB monitors are live; when
// cacheAuthoritative is false (a reconnect in progress, or a suspended snapshot
// session that purged the live cache) those checks are skipped, and OVSDB's
// server-side referential integrity enforces existence at transact time instead
// of the empty cache producing a false 400. Checks that do not touch the cache
// (the ACL priority range) always run.
func ValidateReferences(ctx context.Context, op WriteOperation, spec TableSpec, nbClient client.Client, cacheAuthoritative bool) error {
	if cacheAuthoritative {
		if err := validateUUIDReferences(op, spec, nbClient); err != nil {
			return err
		}
	}
	switch op.Table {
	case "Logical_Switch_Port":
		if cacheAuthoritative {
			return validateLSPReferences(ctx, op, nbClient)
		}
	case "NAT":
		if cacheAuthoritative {
			return validateNATReferences(ctx, op, nbClient)
		}
	case "ACL":
		return validateACLReferences(ctx, op, nbClient)
	}
	return nil
}

// validateUUIDReferences checks that every UUID referenced by op.Fields exists
// in the referenced table's cache, deriving the UUID-typed columns from the NB
// schema instead of a hand-maintained per-table list.
func validateUUIDReferences(op WriteOperation, spec TableSpec, nbClient client.Client) error {
	ts, ok := nbDatabaseSchema.Tables[spec.Table]
	if !ok {
		return nil
	}
	for col, val := range op.Fields {
		cs := ts.Column(col)
		if cs == nil || cs.TypeObj == nil {
			continue
		}
		keyRef := columnRefTable(cs.TypeObj.Key)
		valRef := columnRefTable(cs.TypeObj.Value)
		if keyRef == "" && valRef == "" {
			continue
		}

		switch v := val.(type) {
		case string: // atomic reference (optional *string field)
			if err := checkRefExists(nbClient, keyRef, v, col); err != nil {
				return err
			}
		case []any: // set of references
			if keyRef == "" {
				continue
			}
			for _, e := range v {
				s, ok := e.(string)
				if !ok {
					continue
				}
				if err := checkRefExists(nbClient, keyRef, s, col); err != nil {
					return err
				}
			}
		case map[string]any: // map with references as keys and/or values
			for mk, mv := range v {
				if err := checkRefExists(nbClient, keyRef, mk, col); err != nil {
					return err
				}
				if s, ok := mv.(string); ok {
					if err := checkRefExists(nbClient, valRef, s, col); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// columnRefTable returns the referenced table name if bt is a UUID reference
// type, or "" otherwise.
func columnRefTable(bt *ovsdb.BaseType) string {
	if bt == nil || bt.Type != ovsdb.TypeUUID {
		return ""
	}
	rt, err := bt.RefTable()
	if err != nil {
		return ""
	}
	return rt
}

// checkRefExists reports an InputError if uuid does not name an existing row in
// refTable. An empty refTable (non-reference position) or empty uuid (clearing
// a reference) is a no-op.
func checkRefExists(nbClient client.Client, refTable, uuid, col string) error {
	if refTable == "" || uuid == "" {
		return nil
	}
	rc := nbClient.Cache().Table(refTable)
	if rc == nil || !rc.HasRow(uuid) {
		return &InputError{Message: fmt.Sprintf("referenced %s %q in column %q does not exist", refTable, uuid, col)}
	}
	return nil
}

func validateLSPReferences(ctx context.Context, op WriteOperation, nbClient client.Client) error {
	// Check router-port reference (referenced by name, not UUID).
	if options, ok := op.Fields["options"].(map[string]any); ok {
		if routerPort, ok := options["router-port"].(string); ok && routerPort != "" {
			var lrps []nb.LogicalRouterPort
			err := nbClient.WhereCache(func(lrp *nb.LogicalRouterPort) bool {
				return lrp.Name == routerPort
			}).List(ctx, &lrps)
			if err != nil {
				return fmt.Errorf("listing logical router ports: %w", err)
			}
			if len(lrps) == 0 {
				return &InputError{Message: fmt.Sprintf("referenced router-port %q does not exist", routerPort)}
			}
		}
	}
	return nil
}

func validateNATReferences(ctx context.Context, op WriteOperation, nbClient client.Client) error {
	if op.Action != "create" {
		return nil
	}
	// Check for duplicate external_ip + type combination
	externalIP, _ := op.Fields["external_ip"].(string)
	natType, _ := op.Fields["type"].(string)
	if externalIP != "" && (natType == "dnat" || natType == "dnat_and_snat") {
		var existingNATs []nb.NAT
		err := nbClient.WhereCache(func(n *nb.NAT) bool {
			return n.ExternalIP == externalIP && n.Type == natType
		}).List(ctx, &existingNATs)
		if err != nil {
			return fmt.Errorf("listing NAT entries: %w", err)
		}
		if len(existingNATs) > 0 {
			return &InputError{Message: fmt.Sprintf("NAT entry with external_ip %q and type %q already exists", externalIP, natType)}
		}
	}
	return nil
}

func validateACLReferences(_ context.Context, op WriteOperation, _ client.Client) error {
	// Validate priority range
	if priority, ok := op.Fields["priority"]; ok {
		var p int
		switch v := priority.(type) {
		case float64:
			p = int(v)
		case int:
			p = v
		}
		if p < 0 || p > 32767 {
			return &InputError{Message: fmt.Sprintf("ACL priority must be between 0 and 32767, got %d", p)}
		}
	}
	return nil
}
