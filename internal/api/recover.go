package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// RecoverMiddleware turns a handler panic into a logged, generic 500 instead of
// a dropped connection. Without it a panicking handler kills the connection with
// no response, no log line, and no entry in northwatch_http_requests_total — the
// failure is invisible both to the client and to monitoring.
//
// http.ErrAbortHandler is re-panicked: it is the standard library's way of
// deliberately aborting a response, not a bug to report.
//
// Installing this inside the metrics wrapper is what lets the recovered 500 be
// counted like any other response.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			// The request fields are attacker-controlled, so they are passed as
			// slog attributes (which quote them) rather than interpolated into the
			// message — and without them a panic report is not actionable.
			// #nosec G706 -- structured slog attributes, not an interpolated message
			slog.Error("panic serving request",
				"method", r.Method, "path", r.URL.Path,
				"panic", rec, "stack", string(debug.Stack()))
			// Best-effort: a handler that already wrote a partial response makes
			// this a no-op beyond a "superfluous WriteHeader" log line, which is
			// still better than leaving the client with a silently dropped
			// connection.
			WriteError(w, http.StatusInternalServerError, "internal server error")
		}()
		next.ServeHTTP(w, r)
	})
}
