package write

import (
	"fmt"
	"strings"
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
