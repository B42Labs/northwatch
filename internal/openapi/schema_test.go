package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
)

func TestSchemaFromModel_LogicalSwitch(t *testing.T) {
	schema := SchemaFromModel(nb.LogicalSwitch{})

	require.Equal(t, "object", schema.Type)
	require.NotEmpty(t, schema.Properties)

	// UUID field
	uuid := schema.Properties["_uuid"]
	require.NotNil(t, uuid)
	assert.Equal(t, "string", uuid.Type)
	assert.Equal(t, "uuid", uuid.Format)

	// String field
	name := schema.Properties["name"]
	require.NotNil(t, name)
	assert.Equal(t, "string", name.Type)

	// Slice field (ports is []string)
	ports := schema.Properties["ports"]
	require.NotNil(t, ports)
	assert.Equal(t, "array", ports.Type)
	require.NotNil(t, ports.Items)
	assert.Equal(t, "string", ports.Items.Type)

	// Map field (external_ids is map[string]string)
	eids := schema.Properties["external_ids"]
	require.NotNil(t, eids)
	assert.Equal(t, "object", eids.Type)
	require.NotNil(t, eids.AdditionalProperties)
	assert.Equal(t, "string", eids.AdditionalProperties.Type)

	// Optional pointer field (copp is *string)
	copp := schema.Properties["copp"]
	require.NotNil(t, copp)
	assert.Equal(t, "string", copp.Type)
	assert.True(t, copp.Nullable)
}

func TestSchemaFromModel_IgnoresUntagged(t *testing.T) {
	type NoTags struct {
		Foo string
		Bar string `ovsdb:"bar"`
	}
	schema := SchemaFromModel(NoTags{})
	assert.Len(t, schema.Properties, 1)
	assert.Contains(t, schema.Properties, "bar")
}

func TestSchemaFromModel_Pointer(t *testing.T) {
	schema := SchemaFromModel(&nb.LogicalSwitch{})
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "_uuid")
}

func TestSchemaFromModel_NonStruct(t *testing.T) {
	schema := SchemaFromModel("not a struct")
	assert.Equal(t, "object", schema.Type)
}
