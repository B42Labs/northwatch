package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/telemetry"
	"github.com/b42labs/northwatch/internal/testutil"
)

func TestTelemetryRaftHealth(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/raft-health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body, "nb")
	assert.Contains(t, body, "sb")
}

func TestTelemetryPropagationTimelineAndHeatmap(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	store := telemetry.NewPropagationStore(100, time.Hour)
	now := time.Now().UnixMilli()
	store.Add(telemetry.PropagationEvent{Chassis: "ch1", Hostname: "h1", Generation: 5, LatencyMs: 12, RecordedAt: now})
	store.Add(telemetry.PropagationEvent{Chassis: "ch1", Hostname: "h1", Generation: 6, LatencyMs: 8, RecordedAt: now})
	store.Add(telemetry.PropagationEvent{Chassis: "ch2", Hostname: "h2", Generation: 6, LatencyMs: 20, RecordedAt: now})

	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, nil, store)

	do := func(t *testing.T, url string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	t.Run("timeline all", func(t *testing.T) {
		w := do(t, "/api/v1/telemetry/propagation/timeline")
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, float64(3), body["count"])
	})

	t.Run("timeline filtered and limited", func(t *testing.T) {
		w := do(t, "/api/v1/telemetry/propagation/timeline?chassis=ch1&since=0&limit=1")
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, float64(1), body["count"])
	})

	t.Run("timeline invalid since", func(t *testing.T) {
		w := do(t, "/api/v1/telemetry/propagation/timeline?since=abc")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("timeline invalid limit", func(t *testing.T) {
		w := do(t, "/api/v1/telemetry/propagation/timeline?limit=abc")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// A non-positive limit used to mean "unlimited", handing the client every
	// event the store holds in one response.
	t.Run("timeline non-positive limit is rejected", func(t *testing.T) {
		for _, q := range []string{"?limit=0", "?limit=-1"} {
			w := do(t, "/api/v1/telemetry/propagation/timeline"+q)
			assert.Equal(t, http.StatusBadRequest, w.Code, q)
			assert.Contains(t, w.Body.String(), "positive integer")
		}
	})

	t.Run("timeline limit above the cap is clamped", func(t *testing.T) {
		w := do(t, fmt.Sprintf("/api/v1/telemetry/propagation/timeline?limit=%d", maxTimelineLimit*100))
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, float64(3), body["count"])
	})

	t.Run("heatmap default window", func(t *testing.T) {
		w := do(t, "/api/v1/telemetry/propagation/heatmap")
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Contains(t, body, "chassis")
	})

	t.Run("heatmap explicit since", func(t *testing.T) {
		w := do(t, "/api/v1/telemetry/propagation/heatmap?since=1")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("heatmap invalid since", func(t *testing.T) {
		w := do(t, "/api/v1/telemetry/propagation/heatmap?since=abc")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTelemetrySummary(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	querier := telemetry.NewQuerier(nbClient, sbClient)
	registry := prometheus.NewRegistry()

	mux := http.NewServeMux()
	RegisterTelemetry(mux, querier, registry, nil)

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
	RegisterTelemetry(mux, querier, registry, nil)

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
	RegisterTelemetry(mux, querier, registry, nil)

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
	RegisterTelemetry(mux, querier, registry, nil)

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
	RegisterTelemetry(mux, querier, registry, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "northwatch_ovsdb_connected")
}
