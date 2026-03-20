package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/telemetry"
)

func TestTelemetrySummary(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	registry := prometheus.NewRegistry()

	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, registry)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/summary", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	connected, ok := body["connected"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, connected["nb"])
	assert.Equal(t, true, connected["sb"])
}

func TestTelemetryFlows(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	registry := prometheus.NewRegistry()

	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, registry)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/flows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(0), body["total"])
}

func TestTelemetryPropagation(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	registry := prometheus.NewRegistry()

	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, registry)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/propagation", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTelemetryCluster(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	registry := prometheus.NewRegistry()

	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, registry)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/cluster", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	connected, ok := body["connected"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, connected["nb"])
}

func TestMetricsEndpoint(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	registry := prometheus.NewRegistry()
	collector := telemetry.NewCollector(nbClient, sbClient)
	registry.MustRegister(collector)

	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, registry)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "northwatch_ovsdb_connected")
}
