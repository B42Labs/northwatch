package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/cluster"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/testutil"
)

func TestClusterProxy_UnknownCluster(t *testing.T) {
	reg := cluster.NewRegistry()
	reg.Register("prod", &cluster.Cluster{Name: "prod"})

	mux := http.NewServeMux()
	RegisterClusterProxy(mux, reg, func(subMux *http.ServeMux, c *cluster.Cluster) {
		subMux.HandleFunc("GET /api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"cluster":"` + c.Name + `"}`))
		})
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters/nonexistent/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "unknown cluster")
}

func TestClusterProxy_RoutesToCorrectCluster(t *testing.T) {
	reg := cluster.NewRegistry()
	reg.Register("prod", &cluster.Cluster{Name: "prod"})
	reg.Register("staging", &cluster.Cluster{Name: "staging"})

	mux := http.NewServeMux()
	RegisterClusterProxy(mux, reg, func(subMux *http.ServeMux, c *cluster.Cluster) {
		name := c.Name // capture
		subMux.HandleFunc("GET /api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"cluster":"` + name + `"}`))
		})
	})

	// Test routing to prod
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters/prod/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "prod", body["cluster"])

	// Test routing to staging
	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters/staging/test", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var body2 map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body2))
	assert.Equal(t, "staging", body2["cluster"])
}

func TestClusterProxy_StaleHeader(t *testing.T) {
	nbC := testutil.SetupNBTestClient(t)
	sbC := testutil.SetupSBTestClient(t)
	dbs := &ovndb.OVNDatabases{NB: nbC, SB: sbC}

	reg := cluster.NewRegistry()
	reg.Register("prod", &cluster.Cluster{Name: "prod", DBs: dbs})

	mux := http.NewServeMux()
	RegisterClusterProxy(mux, reg, func(subMux *http.ServeMux, c *cluster.Cluster) {
		subMux.HandleFunc("GET /api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	// Ready cluster: no stale header.
	require.True(t, dbs.Ready())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters/prod/test", nil))
	assert.Empty(t, w.Header().Get("X-Northwatch-Stale"))

	// Not ready (clients closed): stale header set for that cluster.
	dbs.Close()
	require.False(t, dbs.Ready())
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters/prod/test", nil))
	assert.Equal(t, "true", w2.Header().Get("X-Northwatch-Stale"))
}

func TestClusterProxy_DynamicAddRemove(t *testing.T) {
	reg := cluster.NewRegistry()
	mux := http.NewServeMux()
	proxy := RegisterClusterProxy(mux, reg, func(subMux *http.ServeMux, c *cluster.Cluster) {})

	doGet := func(path string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}

	// A snapshot cluster added after startup becomes reachable, then unreachable
	// again once removed — the runtime path a UI-loaded snapshot follows.
	assert.Equal(t, http.StatusNotFound, doGet("/api/v1/clusters/snapshot-1/test"))

	sub := http.NewServeMux()
	sub.HandleFunc("GET /api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	proxy.Add("snapshot-1", sub)
	assert.Equal(t, http.StatusOK, doGet("/api/v1/clusters/snapshot-1/test"))

	proxy.Remove("snapshot-1")
	assert.Equal(t, http.StatusNotFound, doGet("/api/v1/clusters/snapshot-1/test"))
}

func TestClusterProxy_NoConflictWithAPICatchAll(t *testing.T) {
	reg := cluster.NewRegistry()
	mux := http.NewServeMux()

	// Registering both on one mux must not panic: the proxy registers
	// per-method cluster patterns so they don't conflict with the method-specific
	// "GET /api/" etc. that RegisterAPICatchAll installs.
	proxy := RegisterClusterProxy(mux, reg, func(*http.ServeMux, *cluster.Cluster) {})
	RegisterAPICatchAll(mux)

	sub := http.NewServeMux()
	sub.HandleFunc("GET /api/v1/nb/logical-switches", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	sub.HandleFunc("POST /api/v1/debug/trace", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	proxy.Add("snapshot-1", sub)

	do := func(method, path string) int {
		req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}

	// Both GET and POST route through the proxy to the snapshot cluster.
	assert.Equal(t, http.StatusOK, do(http.MethodGet, "/api/v1/clusters/snapshot-1/nb/logical-switches"))
	assert.Equal(t, http.StatusAccepted, do(http.MethodPost, "/api/v1/clusters/snapshot-1/debug/trace"))
	// Unknown API paths still hit the catch-all's JSON 404.
	assert.Equal(t, http.StatusNotFound, do(http.MethodGet, "/api/v1/does-not-exist"))
}

func TestClusterProxy_PreservesQueryParams(t *testing.T) {
	reg := cluster.NewRegistry()
	reg.Register("prod", &cluster.Cluster{Name: "prod"})

	mux := http.NewServeMux()
	RegisterClusterProxy(mux, reg, func(subMux *http.ServeMux, c *cluster.Cluster) {
		subMux.HandleFunc("GET /api/v1/search", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"query":"` + q + `"}`)) //nolint:gosec // test-only handler, q comes from test input
		})
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters/prod/search?q=test-query", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-query", body["query"])
}
