package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding json response failed", "err", err)
	}
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// WriteInternalError is the single policy for server-side failures: the cause is
// logged with the request that triggered it, and the client gets a generic body.
//
// It replaces two habits that were both wrong. Returning err.Error() leaked
// filesystem paths and SQL text to unauthenticated callers; returning a bare
// "internal error" and dropping err made production 500s undebuggable. Every 5xx
// goes through here so neither happens again.
func WriteInternalError(w http.ResponseWriter, r *http.Request, err error) {
	// The request fields are attacker-controlled, so they are passed as slog
	// attributes (which quote them) rather than interpolated into the message —
	// and without them the log entry cannot be tied back to a request.
	// #nosec G706 -- structured slog attributes, not an interpolated message
	slog.Error("internal server error", "method", r.Method, "path", r.URL.Path, "err", err)
	WriteError(w, http.StatusInternalServerError, "internal server error")
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
