package openapi

import (
	"reflect"
	"strings"
)

// SchemaFromModel generates an OpenAPI Schema from an OVSDB model struct
// by walking its ovsdb struct tags, mirroring the same reflection pattern
// used by api.ModelToMap.
func SchemaFromModel(model any) *Schema {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return &Schema{Type: "object"}
	}

	props := make(map[string]*Schema)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("ovsdb")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		props[tag] = goTypeToSchema(field.Type, tag)
	}

	return &Schema{
		Type:       "object",
		Properties: props,
	}
}

func goTypeToSchema(t reflect.Type, tagName string) *Schema {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		inner := goTypeToSchema(t.Elem(), tagName)
		inner.Nullable = true
		return inner
	}

	switch t.Kind() {
	case reflect.String:
		s := &Schema{Type: "string"}
		if tagName == "_uuid" {
			s.Format = "uuid"
		}
		return s
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Slice:
		return &Schema{
			Type:  "array",
			Items: goTypeToSchema(t.Elem(), ""),
		}
	case reflect.Map:
		return &Schema{
			Type:                 "object",
			AdditionalProperties: goTypeToSchema(t.Elem(), ""),
		}
	default:
		return &Schema{Type: "object"}
	}
}
