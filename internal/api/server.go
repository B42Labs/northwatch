package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
)

type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
	dbs        *ovndb.OVNDatabases
}

func NewServer(addr string, dbs *ovndb.OVNDatabases) *Server {
	mux := http.NewServeMux()
	s := &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		mux: mux,
		dbs: dbs,
	}
	return s
}

func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

func (s *Server) Databases() *ovndb.OVNDatabases {
	return s.dbs
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	fmt.Printf("Northwatch listening on %s\n", ln.Addr().String())
	return s.httpServer.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
