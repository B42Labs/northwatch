package handler

import (
	"net/http"
	"net/url"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/cluster"
)

// RegisterClusterProxy registers cluster-prefixed routes that delegate to
// per-cluster sub-muxes. Each cluster gets its own ServeMux with the standard
// /api/v1/... routes. Requests to /api/v1/clusters/{cluster}/{path...} are
// rewritten to /api/v1/{path} and dispatched to the matching cluster's mux.
//
// The registerRoutes callback is called once per cluster to populate each
// sub-mux with that cluster's handlers. This avoids coupling the proxy to
// every individual handler registration function.
func RegisterClusterProxy(mainMux *http.ServeMux, reg *cluster.Registry, registerRoutes func(mux *http.ServeMux, c *cluster.Cluster)) {
	muxMap := make(map[string]*http.ServeMux)

	for _, c := range reg.List() {
		subMux := http.NewServeMux()
		registerRoutes(subMux, c)
		muxMap[c.Name] = subMux
	}

	// Catch-all for /api/v1/clusters/{cluster}/...
	mainMux.HandleFunc("/api/v1/clusters/{cluster}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("cluster")
		subMux, ok := muxMap[name]
		if !ok {
			api.WriteError(w, http.StatusNotFound, "unknown cluster: "+name)
			return
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
	})
}
