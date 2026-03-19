package api

import (
	"encoding/json"
	"log"
	"net/http"
	"reflect"
	"strings"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("WriteJSON: encoding response: %v", err)
	}
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ModelToMap converts an OVSDB model struct to a map using ovsdb struct tags as keys.
func ModelToMap(model any) map[string]any {
	result := make(map[string]any)
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
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
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	result := make([]map[string]any, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = ModelToMap(v.Index(i).Interface())
	}
	return result
}
