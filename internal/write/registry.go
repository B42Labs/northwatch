package write

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
)

// TableSpec describes a writable NB table.
type TableSpec struct {
	Table          string
	ModelType      reflect.Type
	ReadOnlyFields map[string]bool // fields users cannot set
}

// Registry maps table names to their specs.
type Registry struct {
	tables map[string]TableSpec
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tables: make(map[string]TableSpec)}
}

// Register adds a writable table spec.
func (r *Registry) Register(spec TableSpec) {
	r.tables[spec.Table] = spec
}

// Get returns the spec for a table, or an error if not writable.
func (r *Registry) Get(table string) (TableSpec, error) {
	spec, ok := r.tables[table]
	if !ok {
		return TableSpec{}, fmt.Errorf("table %q is not writable", table)
	}
	return spec, nil
}

// Tables returns all registered table names.
func (r *Registry) Tables() []string {
	names := make([]string, 0, len(r.tables))
	for name := range r.tables {
		names = append(names, name)
	}
	return names
}

// RegisterModel is a helper to register a model type with standard read-only fields.
func RegisterModel[T any](r *Registry, table string, extraReadOnly ...string) {
	var zero T
	readOnly := map[string]bool{"_uuid": true}
	for _, f := range extraReadOnly {
		readOnly[f] = true
	}
	r.Register(TableSpec{
		Table:          table,
		ModelType:      reflect.TypeOf(zero),
		ReadOnlyFields: readOnly,
	})
}

// FieldInfo describes a single field in a writable table.
type FieldInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Optional bool   `json:"optional"`
	ReadOnly bool   `json:"read_only"`
}

// TableSchema describes a writable table and its fields.
type TableSchema struct {
	Table  string      `json:"table"`
	Fields []FieldInfo `json:"fields"`
}

// Schema returns the schema for all registered writable tables.
func (r *Registry) Schema() []TableSchema {
	schemas := make([]TableSchema, 0, len(r.tables))
	for _, spec := range r.tables {
		schemas = append(schemas, tableSchema(spec))
	}
	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i].Table < schemas[j].Table
	})
	return schemas
}

func tableSchema(spec TableSpec) TableSchema {
	t := spec.ModelType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	var fields []FieldInfo
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		tag := sf.Tag.Get("ovsdb")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		fi := FieldInfo{
			Name:     tag,
			Type:     goTypeToOVSDB(sf.Type),
			Optional: sf.Type.Kind() == reflect.Ptr,
			ReadOnly: spec.ReadOnlyFields[tag],
		}
		fields = append(fields, fi)
	}
	return TableSchema{Table: spec.Table, Fields: fields}
}

func goTypeToOVSDB(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		return goTypeToOVSDB(t.Elem())
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "integer"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice:
		return "set<" + goTypeToOVSDB(t.Elem()) + ">"
	case reflect.Map:
		return "map<" + goTypeToOVSDB(t.Key()) + "," + goTypeToOVSDB(t.Elem()) + ">"
	default:
		return "string"
	}
}

// OVSDBFieldNames returns all ovsdb tag names for a model type.
func OVSDBFieldNames(t reflect.Type) []string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	var names []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("ovsdb")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		names = append(names, tag)
	}
	return names
}

// DefaultRegistry returns a Registry pre-populated with the initial writable NB tables.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	RegisterModel[nb.LogicalSwitch](r, "Logical_Switch")
	RegisterModel[nb.LogicalSwitchPort](r, "Logical_Switch_Port")
	RegisterModel[nb.LogicalRouter](r, "Logical_Router")
	RegisterModel[nb.LogicalRouterPort](r, "Logical_Router_Port")
	RegisterModel[nb.ACL](r, "ACL")
	RegisterModel[nb.NAT](r, "NAT")
	RegisterModel[nb.AddressSet](r, "Address_Set")
	RegisterModel[nb.PortGroup](r, "Port_Group")
	RegisterModel[nb.LoadBalancer](r, "Load_Balancer")
	RegisterModel[nb.LogicalRouterStaticRoute](r, "Logical_Router_Static_Route")
	RegisterModel[nb.LogicalRouterPolicy](r, "Logical_Router_Policy")
	RegisterModel[nb.DHCPOptions](r, "DHCP_Options")
	RegisterModel[nb.DNS](r, "DNS")
	RegisterModel[nb.StaticMACBinding](r, "Static_MAC_Binding")
	return r
}
