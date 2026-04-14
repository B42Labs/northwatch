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

