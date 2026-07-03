// Package handler holds Northwatch's HTTP route handlers, one file per feature
// area.
//
// Each file exposes a RegisterX(mux, deps...) function that registers its routes
// on the provided *http.ServeMux using Go 1.22 method+pattern syntax
// (e.g. "GET /api/v1/{db}/{table}" and "GET /api/v1/{db}/{table}/{uuid}"). The
// server wiring calls these Register* functions to assemble the full API
// surface; because {uuid}/{name} path wildcards never match an empty segment, a
// handler reading r.PathValue never sees an empty value.
//
// Error policy: handlers reply with api.WriteError and an appropriate 4xx status
// for client faults (malformed input, not found) and a generic 500 for server
// faults, without leaking internal detail. List responses go through
// api.WriteJSONList / api.NonNil so an empty result marshals as [] rather than
// null.
package handler
