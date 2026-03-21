package write

import (
	"reflect"
	"testing"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapToModel_LogicalSwitch_StringField(t *testing.T) {
	fields := map[string]any{
		"name": "my-switch",
	}
	result, err := MapToModel(fields, reflect.TypeOf(nb.LogicalSwitch{}))
	require.NoError(t, err)

	ls, ok := result.(*nb.LogicalSwitch)
	require.True(t, ok)
	assert.Equal(t, "my-switch", ls.Name)
}

func TestMapToModel_LogicalSwitch_MapField(t *testing.T) {
	fields := map[string]any{
		"name":         "sw1",
		"external_ids": map[string]any{"env": "prod"},
	}
	result, err := MapToModel(fields, reflect.TypeOf(nb.LogicalSwitch{}))
	require.NoError(t, err)

	ls, ok := result.(*nb.LogicalSwitch)
	require.True(t, ok)
	assert.Equal(t, "sw1", ls.Name)
	assert.Equal(t, map[string]string{"env": "prod"}, ls.ExternalIDs)
}

func TestMapToModel_LogicalSwitch_SliceField(t *testing.T) {
	fields := map[string]any{
		"name": "sw2",
		"acls": []any{"uuid1", "uuid2"},
	}
	result, err := MapToModel(fields, reflect.TypeOf(nb.LogicalSwitch{}))
	require.NoError(t, err)

	ls, ok := result.(*nb.LogicalSwitch)
	require.True(t, ok)
	assert.Equal(t, []string{"uuid1", "uuid2"}, ls.ACLs)
}

func TestMapToModel_LogicalSwitch_PointerStringField(t *testing.T) {
	fields := map[string]any{
		"name": "sw3",
		"copp": "copp-uuid",
	}
	result, err := MapToModel(fields, reflect.TypeOf(nb.LogicalSwitch{}))
	require.NoError(t, err)

	ls, ok := result.(*nb.LogicalSwitch)
	require.True(t, ok)
	expected := "copp-uuid"
	assert.Equal(t, &expected, ls.Copp)
}

func TestMapToModel_ACL_IntAndBoolFields(t *testing.T) {
	fields := map[string]any{
		"action":    "allow",
		"direction": "from-lport",
		"match":     "ip4",
		"priority":  float64(100),
		"log":       true,
	}
	result, err := MapToModel(fields, reflect.TypeOf(nb.ACL{}))
	require.NoError(t, err)

	acl, ok := result.(*nb.ACL)
	require.True(t, ok)
	assert.Equal(t, "allow", acl.Action)
	assert.Equal(t, "from-lport", acl.Direction)
	assert.Equal(t, "ip4", acl.Match)
	assert.Equal(t, 100, acl.Priority)
	assert.True(t, acl.Log)
}

func TestMapToModel_UnknownField(t *testing.T) {
	fields := map[string]any{
		"nonexistent_field": "value",
	}
	_, err := MapToModel(fields, reflect.TypeOf(nb.LogicalSwitch{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestMapToModel_TypeMismatch(t *testing.T) {
	fields := map[string]any{
		"name": 42, // should be string
	}
	_, err := MapToModel(fields, reflect.TypeOf(nb.LogicalSwitch{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

func TestMapToModel_EmptyFields(t *testing.T) {
	fields := map[string]any{}
	result, err := MapToModel(fields, reflect.TypeOf(nb.LogicalSwitch{}))
	require.NoError(t, err)

	ls, ok := result.(*nb.LogicalSwitch)
	require.True(t, ok)
	assert.Equal(t, "", ls.Name)
}
