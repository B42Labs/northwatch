package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_AddOperation(t *testing.T) {
	b := NewBuilder()
	b.AddOperation("/test", "get", &Operation{
		OperationID: "testOp",
		Summary:     "Test operation",
		Responses:   jsonOK("OK"),
	})

	doc := b.Build()
	require.Contains(t, doc.Paths, "/test")
	require.NotNil(t, doc.Paths["/test"].Get)
	assert.Equal(t, "testOp", doc.Paths["/test"].Get.OperationID)
}

func TestBuilder_AddOperation_MultipleMethodsSamePath(t *testing.T) {
	b := NewBuilder()
	b.AddOperation("/items", "get", &Operation{OperationID: "listItems", Responses: jsonOK("OK")})
	b.AddOperation("/items", "post", &Operation{OperationID: "createItem", Responses: jsonCreated("Created")})

	doc := b.Build()
	require.Contains(t, doc.Paths, "/items")
	assert.NotNil(t, doc.Paths["/items"].Get)
	assert.NotNil(t, doc.Paths["/items"].Post)
}

func TestBuilder_AddSchema(t *testing.T) {
	b := NewBuilder()
	ref := b.AddSchema("Foo", &Schema{Type: "object"})

	assert.Equal(t, "#/components/schemas/Foo", ref)

	doc := b.Build()
	require.Contains(t, doc.Components.Schemas, "Foo")
	assert.Equal(t, "object", doc.Components.Schemas["Foo"].Type)
}

func TestAddTableEndpoints(t *testing.T) {
	type TestModel struct {
		UUID string `ovsdb:"_uuid"`
		Name string `ovsdb:"name"`
	}

	b := NewBuilder()
	AddTableEndpoints(b, TestModel{}, "/api/v1/test/items", "Test", "TestModel")

	doc := b.Build()

	// List endpoint
	require.Contains(t, doc.Paths, "/api/v1/test/items")
	require.NotNil(t, doc.Paths["/api/v1/test/items"].Get)
	assert.Equal(t, "listTestModel", doc.Paths["/api/v1/test/items"].Get.OperationID)

	// Detail endpoint
	require.Contains(t, doc.Paths, "/api/v1/test/items/{uuid}")
	require.NotNil(t, doc.Paths["/api/v1/test/items/{uuid}"].Get)
	assert.Equal(t, "getTestModel", doc.Paths["/api/v1/test/items/{uuid}"].Get.OperationID)

	// Schema component
	require.Contains(t, doc.Components.Schemas, "TestModel")
	schema := doc.Components.Schemas["TestModel"]
	assert.Contains(t, schema.Properties, "_uuid")
	assert.Contains(t, schema.Properties, "name")
}
