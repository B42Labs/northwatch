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
)

func TestClusters_Empty(t *testing.T) {
	reg := cluster.NewRegistry()

	mux := http.NewServeMux()
	RegisterClusters(mux, reg)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	clusters, ok := body["clusters"].([]any)
	require.True(t, ok)
	assert.Empty(t, clusters)
}

func TestClusters_WithClusters(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	// The in-memory test clients report Connected() = true, so Ready() = true
	dbs := &ovndb.OVNDatabases{NB: nbClient, SB: sbClient}

	reg := cluster.NewRegistry()
	reg.Register("prod", &cluster.Cluster{
		Name:  "prod",
		Label: "Production",
		DBs:   dbs,
	})
	reg.Register("staging", &cluster.Cluster{
		Name:  "staging",
		Label: "Staging",
		DBs:   dbs,
	})

	mux := http.NewServeMux()
	RegisterClusters(mux, reg)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	clusters, ok := body["clusters"].([]any)
	require.True(t, ok)
	require.Len(t, clusters, 2)

	c0 := clusters[0].(map[string]any)
	assert.Equal(t, "prod", c0["name"])
	assert.Equal(t, "Production", c0["label"])

	c1 := clusters[1].(map[string]any)
	assert.Equal(t, "staging", c1["name"])
	assert.Equal(t, "Staging", c1["label"])
}

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
