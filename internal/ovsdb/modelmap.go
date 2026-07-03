package ovsdb

import (
	"reflect"
	"strings"
)

// ModelToMap converts an OVSDB model struct to a map using ovsdb struct tags as
// keys. Fields without an ovsdb tag (or tagged "-") are skipped.
func ModelToMap(model any) map[string]any {
	result := make(map[string]any)
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("ovsdb")
		if tag == "" || tag == "-" {
			continue
		}
		// Strip any tag options after comma
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		result[tag] = v.Field(i).Interface()
	}
	return result
}

// ModelsToMaps converts a slice of OVSDB model structs to a slice of maps.
func ModelsToMaps(models any) []map[string]any {
	v := reflect.ValueOf(models)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	result := make([]map[string]any, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = ModelToMap(v.Index(i).Interface())
	}
	return result
}

// DerefStr returns the value pointed to by s, or the empty string when s is nil.
// It is a convenience for the many OVSDB optional string columns modeled as
// *string.
func DerefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
