package write

import (
	"testing"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRegistry() *Registry {
	r := NewRegistry()
	RegisterModel[nb.LogicalSwitch](r, "Logical_Switch")
	RegisterModel[nb.ACL](r, "ACL")
	return r
}

func TestValidateOperation_ValidCreate(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "create",
		Table:  "Logical_Switch",
		Fields: map[string]any{"name": "test-switch"},
	}
	err := ValidateOperation(op, reg)
	assert.NoError(t, err)
}

func TestValidateOperation_ValidUpdate(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "update",
		Table:  "Logical_Switch",
		UUID:   "some-uuid",
		Fields: map[string]any{"name": "updated-switch"},
	}
	err := ValidateOperation(op, reg)
	assert.NoError(t, err)
}

func TestValidateOperation_ValidDelete(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "delete",
		Table:  "Logical_Switch",
		UUID:   "some-uuid",
	}
	err := ValidateOperation(op, reg)
	assert.NoError(t, err)
}

func TestValidateOperation_InvalidAction(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "upsert",
		Table:  "Logical_Switch",
	}
	err := ValidateOperation(op, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid action")
}

func TestValidateOperation_UnwritableTable(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "create",
		Table:  "NB_Global",
		Fields: map[string]any{"name": "x"},
	}
	err := ValidateOperation(op, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not writable")
}

func TestValidateOperation_CreateWithUUID(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "create",
		Table:  "Logical_Switch",
		UUID:   "should-not-be-here",
		Fields: map[string]any{"name": "test"},
	}
	err := ValidateOperation(op, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not specify a UUID")
}

func TestValidateOperation_UpdateWithoutUUID(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "update",
		Table:  "Logical_Switch",
		Fields: map[string]any{"name": "test"},
	}
	err := ValidateOperation(op, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require a UUID")
}

func TestValidateOperation_DeleteWithoutUUID(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "delete",
		Table:  "Logical_Switch",
	}
	err := ValidateOperation(op, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require a UUID")
}

func TestValidateOperation_CreateWithoutFields(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "create",
		Table:  "Logical_Switch",
	}
	err := ValidateOperation(op, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require at least one field")
}

func TestValidateOperation_ReadOnlyField(t *testing.T) {
	reg := testRegistry()
	op := WriteOperation{
		Action: "create",
		Table:  "Logical_Switch",
		Fields: map[string]any{"_uuid": "some-uuid", "name": "test"},
	}
	err := ValidateOperation(op, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")
}

func TestValidateFields_UnknownField(t *testing.T) {
	reg := testRegistry()
	spec, err := reg.Get("Logical_Switch")
	require.NoError(t, err)

	fields := map[string]any{
		"nonexistent": "value",
	}
	err = ValidateFields(fields, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestValidateFields_ValidFields(t *testing.T) {
	reg := testRegistry()
	spec, err := reg.Get("Logical_Switch")
	require.NoError(t, err)

	fields := map[string]any{
		"name":         "my-switch",
		"external_ids": map[string]string{"key": "val"},
	}
	err = ValidateFields(fields, spec)
	assert.NoError(t, err)
}
