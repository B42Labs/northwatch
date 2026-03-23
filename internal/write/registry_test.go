package write

import (
	"sort"
	"testing"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	RegisterModel[nb.LogicalSwitch](r, "Logical_Switch")

	spec, err := r.Get("Logical_Switch")
	require.NoError(t, err)
	assert.Equal(t, "Logical_Switch", spec.Table)
	assert.True(t, spec.ReadOnlyFields["_uuid"])
}

func TestRegistry_GetUnregistered(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("Nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not writable")
}

func TestRegistry_Tables(t *testing.T) {
	r := NewRegistry()
	RegisterModel[nb.LogicalSwitch](r, "Logical_Switch")
	RegisterModel[nb.ACL](r, "ACL")

	tables := r.Tables()
	sort.Strings(tables)
	assert.Equal(t, []string{"ACL", "Logical_Switch"}, tables)
}

func TestRegistry_ExtraReadOnly(t *testing.T) {
	r := NewRegistry()
	RegisterModel[nb.LogicalSwitchPort](r, "Logical_Switch_Port", "up")

	spec, err := r.Get("Logical_Switch_Port")
	require.NoError(t, err)
	assert.True(t, spec.ReadOnlyFields["_uuid"])
	assert.True(t, spec.ReadOnlyFields["up"])
}

func TestOVSDBFieldNames(t *testing.T) {
	reg := testRegistry()
	spec, err := reg.Get("Logical_Switch")
	require.NoError(t, err)

	fields := OVSDBFieldNames(spec.ModelType)
	assert.Contains(t, fields, "_uuid")
	assert.Contains(t, fields, "name")
	assert.Contains(t, fields, "external_ids")
	assert.Contains(t, fields, "ports")
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()

	expectedTables := []string{
		"Logical_Switch", "Logical_Switch_Port",
		"Logical_Router", "Logical_Router_Port",
		"ACL", "NAT", "Address_Set", "Port_Group",
		"Load_Balancer", "Logical_Router_Static_Route",
		"Logical_Router_Policy", "DHCP_Options", "DNS",
		"Static_MAC_Binding", "HA_Chassis", "Gateway_Chassis",
	}

	tables := r.Tables()
	sort.Strings(tables)
	sort.Strings(expectedTables)
	assert.Equal(t, expectedTables, tables)
}
