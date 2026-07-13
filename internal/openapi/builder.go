package openapi

import "fmt"

// Builder accumulates OpenAPI paths and component schemas.
type Builder struct {
	doc Document
}

// NewBuilder creates a new Builder with initialized document structure.
func NewBuilder() *Builder {
	return &Builder{
		doc: Document{
			OpenAPI: "3.1.0",
			Info: Info{
				Title:       "Northwatch API",
				Description: "REST API for browsing, debugging, and monitoring OVN deployments",
				Version:     "1.0.0",
			},
			Paths: make(map[string]*PathItem),
			Components: &Components{
				Schemas: make(map[string]*Schema),
				SecuritySchemes: map[string]*SecurityScheme{
					bearerScheme: {
						Type:        "http",
						Scheme:      "bearer",
						Description: "API token configured with --api-tokens. Required on every mutating request; read endpoints are open.",
					},
				},
			},
		},
	}
}

// bearerScheme is the name of the security scheme every mutating operation
// requires.
const bearerScheme = "bearerAuth"

// AddOperation registers a single operation on a path.
//
// Every non-GET operation is marked as requiring the bearer token and gains a
// 401 response. That mirrors exactly what api.AuthMiddleware enforces at
// runtime — deriving it from the method rather than annotating each operation by
// hand means a new mutating route cannot be added to the spec without its
// security requirement.
func (b *Builder) AddOperation(path, method string, op *Operation) {
	if method != "get" {
		op.Security = []map[string][]string{{bearerScheme: {}}}
		if op.Responses == nil {
			op.Responses = map[string]Response{}
		}
		op.Responses["401"] = Response{Description: "Authentication required"}
	}

	pi, ok := b.doc.Paths[path]
	if !ok {
		pi = &PathItem{}
		b.doc.Paths[path] = pi
	}
	switch method {
	case "get":
		pi.Get = op
	case "post":
		pi.Post = op
	case "put":
		pi.Put = op
	case "delete":
		pi.Delete = op
	default:
		// Spec building runs at init time, so a mistyped or unsupported method
		// must fail loudly (in tests/CI) rather than silently drop the operation
		// and diverge the spec from the live routes.
		panic(fmt.Sprintf("openapi: unknown method %q for %s", method, path))
	}
}

// AddSchema registers a component schema and returns its $ref string.
func (b *Builder) AddSchema(name string, schema *Schema) string {
	b.doc.Components.Schemas[name] = schema
	return fmt.Sprintf("#/components/schemas/%s", name)
}

// Build returns the completed Document.
func (b *Builder) Build() Document {
	return b.doc
}

// AddTableEndpoints registers list and get-by-uuid paths for an OVSDB model,
// mirroring handler.registerTable[T].
func AddTableEndpoints(b *Builder, model any, basePath, tag, schemaName string) {
	schema := SchemaFromModel(model)
	ref := b.AddSchema(schemaName, schema)

	// List
	b.AddOperation(basePath, "get", &Operation{
		OperationID: "list" + schemaName,
		Summary:     "List " + schemaName + " rows",
		Tags:        []string{tag},
		Parameters:  paginationParams(),
		Responses: map[string]Response{
			"200": {
				Description: "Array of " + schemaName,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{
						Type:  "array",
						Items: &Schema{Ref: ref},
					}},
				},
			},
		},
	})

	// Get by UUID
	b.AddOperation(basePath+"/{uuid}", "get", &Operation{
		OperationID: "get" + schemaName,
		Summary:     "Get " + schemaName + " by UUID",
		Tags:        []string{tag},
		Parameters: []Parameter{
			{Name: "uuid", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Single " + schemaName,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{Ref: ref}},
				},
			},
			"404": {Description: "Not found"},
		},
	})
}

// jsonOK returns a standard 200 JSON response.
func jsonOK(description string) map[string]Response {
	return map[string]Response{
		"200": {
			Description: description,
			Content: map[string]MediaType{
				"application/json": {Schema: &Schema{Type: "object"}},
			},
		},
	}
}

// jsonCreated returns a standard 201 JSON response.
func jsonCreated(description string) map[string]Response {
	return map[string]Response{
		"201": {
			Description: description,
			Content: map[string]MediaType{
				"application/json": {Schema: &Schema{Type: "object"}},
			},
		},
	}
}

// noContent returns a standard 204 response.
func noContent() map[string]Response {
	return map[string]Response{
		"204": {Description: "No content"},
	}
}

// paginationParams describes the limit/offset window every list endpoint
// applies. Responses carry X-Total-Count, and X-Truncated when rows remain.
func paginationParams() []Parameter {
	return []Parameter{
		{
			Name: "limit", In: "query",
			Description: "Maximum rows to return (default and hard maximum: 5000)",
			Schema:      &Schema{Type: "integer"},
		},
		{
			Name: "offset", In: "query",
			Description: "Rows to skip, ordered by UUID (default: 0)",
			Schema:      &Schema{Type: "integer"},
		},
	}
}

func queryParam(name, description string) Parameter {
	return Parameter{Name: name, In: "query", Description: description, Schema: &Schema{Type: "string"}}
}

func requiredQueryParam(name, description string) Parameter {
	return Parameter{Name: name, In: "query", Required: true, Description: description, Schema: &Schema{Type: "string"}}
}

func pathParam(name string) Parameter {
	return Parameter{Name: name, In: "path", Required: true, Schema: &Schema{Type: "string"}}
}

func jsonBody() *RequestBody {
	return &RequestBody{
		Required: true,
		Content: map[string]MediaType{
			"application/json": {Schema: &Schema{Type: "object"}},
		},
	}
}
