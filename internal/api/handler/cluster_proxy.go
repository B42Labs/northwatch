package handler

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/cluster"
)

// ClusterProxy dispatches /api/v1/clusters/{cluster}/{path...} requests to a
// per-cluster sub-mux. Each cluster gets its own ServeMux with the standard
// /api/v1/... routes; requests are rewritten to /api/v1/{path} before dispatch.
//
// The sub-mux map is guarded by a RWMutex so clusters can be added and removed
// at runtime — this is what lets a snapshot loaded from the UI become a live,
// switchable data source without a restart.
type ClusterProxy struct {
	mu     sync.RWMutex
	muxMap map[string]*http.ServeMux
}

// Add registers (or replaces) the sub-mux serving a cluster's routes.
func (p *ClusterProxy) Add(name string, mux *http.ServeMux) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.muxMap[name] = mux
}

// Remove drops a cluster's sub-mux. In-flight requests already dispatched run to
// completion; subsequent requests to the cluster return 404.
func (p *ClusterProxy) Remove(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.muxMap, name)
}

func (p *ClusterProxy) lookup(name string) (*http.ServeMux, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	m, ok := p.muxMap[name]
	return m, ok
}

// RegisterClusterProxy installs the cluster-prefixed catch-all route on mainMux
// and returns the proxy handle so clusters can be added/removed at runtime.
//
// Every cluster currently in reg is registered up front via the registerRoutes
// callback, which populates each sub-mux with that cluster's handlers. The
// callback keeps the proxy decoupled from individual handler registration.
func RegisterClusterProxy(mainMux *http.ServeMux, reg *cluster.Registry, registerRoutes func(mux *http.ServeMux, c *cluster.Cluster)) *ClusterProxy {
	p := &ClusterProxy{muxMap: make(map[string]*http.ServeMux)}

	for _, c := range reg.List() {
		subMux := http.NewServeMux()
		registerRoutes(subMux, c)
		p.Add(c.Name, subMux)
	}

	dispatch := func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("cluster")
		subMux, ok := p.lookup(name)
		if !ok {
			api.WriteError(w, http.StatusNotFound, "unknown cluster: "+name)
			return
		}

		// Mark the response stale when this cluster's backing databases are not
		// ready (disconnected/reconnecting, or suspended by a snapshot session).
		if c, ok := reg.Get(name); ok && c.DBs != nil && !c.DBs.Ready() {
			w.Header().Set("X-Northwatch-Stale", "true")
		}

		// Rewrite path: /api/v1/clusters/prod/nb/... -> /api/v1/nb/...
		rest := r.PathValue("path")
		newPath := "/api/v1/" + rest

		// Copy the URL to avoid mutating the original request.
		u := *r.URL
		u.Path = newPath
		u.RawPath = ""

		r2 := new(http.Request)
		*r2 = *r
		r2.URL = &url.URL{}
		*r2.URL = u
		r2.RequestURI = newPath
		if r.URL.RawQuery != "" {
			r2.RequestURI = newPath + "?" + r.URL.RawQuery
		}

		subMux.ServeHTTP(w, r2)
	}

	// Register per method rather than method-less. The API catch-all
	// (RegisterAPICatchAll) registers method-specific "GET /api/" etc.; a
	// method-less "/api/v1/clusters/{cluster}/{path...}" would be incomparable to
	// those (more specific path, but fewer methods) and ServeMux would panic on
	// the conflict. Per-method patterns share each method with the catch-all and
	// win purely on path specificity.
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		mainMux.HandleFunc(m+" /api/v1/clusters/{cluster}/{path...}", dispatch)
	}

	return p
}
