package api

import (
	"net/http"
	"strings"
	"time"
)

// DeadlineMiddleware gives every request a write deadline, bounding how long a
// slow-reading client can hold a connection open. It exists because the server
// cannot simply set http.Server.WriteTimeout: that deadline also applies to the
// WebSocket endpoint, whose connections are long-lived by design and would be
// torn down mid-stream.
//
// WebSocket requests therefore get both deadlines cleared instead. Their
// liveness is managed by the WebSocket handler's own ping/pong ticker and
// per-write timeouts (see handler.RegisterWS), which is a tighter bound than a
// blanket connection deadline.
//
// A ResponseWriter that does not support deadlines (an httptest recorder, a
// wrapper that hides the seam) makes both calls no-ops, which is why the
// controller's errors are ignored.
func DeadlineMiddleware(writeTimeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			if isWebSocketPath(r.URL.Path) {
				_ = rc.SetReadDeadline(time.Time{})
				_ = rc.SetWriteDeadline(time.Time{})
			} else {
				_ = rc.SetWriteDeadline(time.Now().Add(writeTimeout))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isWebSocketPath reports whether path is the event-stream endpoint, either at
// the top level or behind the cluster proxy (/api/v1/clusters/{name}/ws).
func isWebSocketPath(path string) bool {
	return path == "/api/v1/ws" ||
		(strings.HasPrefix(path, "/api/v1/clusters/") && strings.HasSuffix(path, "/ws"))
}
