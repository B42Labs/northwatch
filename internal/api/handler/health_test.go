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

type mockDatabases struct {
	ready bool
}

func (m *mockDatabases) Ready() bool { return m.ready }

func TestHealthz(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
}

func TestReadyz_NotReady(t *testing.T) {
	mux := http.NewServeMux()
	dbs := &mockDatabases{ready: false}
	mux.HandleFunc("GET /readyz", handleReadyzFunc(dbs))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestReadyz_Ready(t *testing.T) {
	mux := http.NewServeMux()
	dbs := &mockDatabases{ready: true}
	mux.HandleFunc("GET /readyz", handleReadyzFunc(dbs))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ready", body["status"])
}
