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
	RegisterCapabilities(mux, false)

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
}

func TestCapabilities_WithEnrichment(t *testing.T) {
	mux := http.NewServeMux()
	RegisterCapabilities(mux, true)

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
