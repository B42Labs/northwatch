package api

import (
	"encoding/json"
	"log"
	"net/http"
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

// WriteJSONList writes items as a JSON array with the given status, coercing a
// nil slice to an empty array so clients always receive [] rather than null.
func WriteJSONList[T any](w http.ResponseWriter, status int, items []T) {
	WriteJSON(w, status, NonNil(items))
}

// NonNil returns s unchanged, or an empty (non-nil) slice when s is nil, so it
// marshals as [] rather than null. Use it for a slice embedded in a larger
// response object, where WriteJSONList cannot be applied directly.
func NonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
