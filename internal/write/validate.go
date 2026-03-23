package write

import (
	"context"
	"fmt"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

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
func ValidateReferences(ctx context.Context, op WriteOperation, nbClient client.Client) error {
	switch op.Table {
	case "Logical_Switch_Port":
		return validateLSPReferences(ctx, op, nbClient)
	case "NAT":
		return validateNATReferences(ctx, op, nbClient)
	case "ACL":
		return validateACLReferences(ctx, op, nbClient)
	}
	return nil
}

func validateLSPReferences(ctx context.Context, op WriteOperation, nbClient client.Client) error {
	// Check router-port reference
	if options, ok := op.Fields["options"].(map[string]any); ok {
		if routerPort, ok := options["router-port"].(string); ok && routerPort != "" {
			var lrps []nb.LogicalRouterPort
			err := nbClient.WhereCache(func(lrp *nb.LogicalRouterPort) bool {
				return lrp.Name == routerPort
			}).List(ctx, &lrps)
			if err != nil || len(lrps) == 0 {
				return fmt.Errorf("referenced router-port %q does not exist", routerPort)
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
		if err == nil && len(existingNATs) > 0 {
			return fmt.Errorf("NAT entry with external_ip %q and type %q already exists", externalIP, natType)
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
			return fmt.Errorf("ACL priority must be between 0 and 32767, got %d", p)
		}
	}
	return nil
}
