package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilities(t *testing.T) {
	mux := http.NewServeMux()
	RegisterCapabilities(mux, false, false, false, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/capabilities", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	caps, ok := body["capabilities"].([]any)
	require.True(t, ok)
	assert.Contains(t, caps, "read")
	assert.Contains(t, caps, "debug")
	assert.Contains(t, caps, "correlate")
	assert.NotContains(t, caps, "enrich")
	assert.NotContains(t, caps, "multi-cluster")
	assert.NotContains(t, caps, "snapshot")
	assert.Equal(t, "live", body["mode"])
	assert.NotContains(t, body, "snapshot")
}

func TestCapabilities_WithSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	RegisterCapabilities(mux, false, false, false, &SnapshotInfo{
		CreatedAt: "2026-06-23T17:27:28Z",
		NBAddr:    "tcp:10.0.0.1:6641",
		SBAddr:    "tcp:10.0.0.1:6642",
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/capabilities", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	caps, ok := body["capabilities"].([]any)
	require.True(t, ok)
	assert.Contains(t, caps, "snapshot")
	assert.Equal(t, "snapshot", body["mode"])
	snap, ok := body["snapshot"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2026-06-23T17:27:28Z", snap["createdAt"])
	assert.Equal(t, "tcp:10.0.0.1:6641", snap["nbAddr"])
}

func TestCapabilities_WithEnrichment(t *testing.T) {
	mux := http.NewServeMux()
	RegisterCapabilities(mux, true, false, false, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/capabilities", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	caps, ok := body["capabilities"].([]any)
	require.True(t, ok)
	assert.Contains(t, caps, "read")
	assert.Contains(t, caps, "debug")
	assert.Contains(t, caps, "correlate")
	assert.Contains(t, caps, "enrich")
}

func TestCapabilities_WithMultiCluster(t *testing.T) {
	mux := http.NewServeMux()
	RegisterCapabilities(mux, false, false, true, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/capabilities", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	caps, ok := body["capabilities"].([]any)
	require.True(t, ok)
	assert.Contains(t, caps, "multi-cluster")
}
