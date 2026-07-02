package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
)

// staleHeader is set on responses whose backing OVSDB caches are not currently
// authoritative (the client is disconnected/reconnecting, or a snapshot session
// has suspended the live monitors). Consumers can surface a staleness warning
// instead of trusting a silently-frozen cache.
const staleHeader = "X-Northwatch-Stale"

// staleHeaderMiddleware marks default-cluster /api/v1 responses as stale when
// ready reports false. Cluster-scoped routes (/api/v1/clusters/...) are
// attributed per cluster by the proxy, so they are skipped here. A nil ready
// predicate disables the marker.
func staleHeaderMiddleware(ready func() bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ready != nil &&
			strings.HasPrefix(r.URL.Path, "/api/v1/") &&
			!strings.HasPrefix(r.URL.Path, "/api/v1/clusters/") &&
			!ready() {
			w.Header().Set(staleHeader, "true")
		}
		next.ServeHTTP(w, r)
	})
}

// Server is the Northwatch HTTP server. It owns the http.ServeMux that
// individual handler packages register routes against.
type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
	dbs        *ovndb.OVNDatabases
}

// NewServer creates a new HTTP server. Optional handler wrappers can be
// provided to instrument the mux (e.g. with metrics middleware).
func NewServer(addr string, dbs *ovndb.OVNDatabases, wrappers ...func(http.Handler) http.Handler) *Server {
	mux := http.NewServeMux()
	// Wrap the mux innermost so the stale marker is set before any caller
	// wrapper (e.g. metrics) runs.
	var ready func() bool
	if dbs != nil {
		ready = dbs.Ready
	}
	handler := staleHeaderMiddleware(ready, mux)
	for _, wrap := range wrappers {
		handler = wrap(handler)
	}
	s := &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
		mux: mux,
		dbs: dbs,
	}
	return s
}

// Mux returns the underlying http.ServeMux so route handlers can register on it.
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// Databases returns the OVN database client bundle the server was created with.
func (s *Server) Databases() *ovndb.OVNDatabases {
	return s.dbs
}

// ListenAndServe binds the listener using ctx for control of the bind itself
// and then begins serving HTTP requests. Use Shutdown to stop the server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	fmt.Printf("Northwatch listening on %s\n", ln.Addr().String())
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully stops the HTTP server, waiting up to ctx's deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
