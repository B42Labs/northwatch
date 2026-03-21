package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/write"

	_ "modernc.org/sqlite"
)

func setupTestWriteEngine(t *testing.T) *write.Engine {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	auditStore, err := write.NewAuditStore(db)
	require.NoError(t, err)

	registry := write.DefaultRegistry()
	// Pass nil nbClient and nil collector — tests that need them will fail gracefully.
	return write.NewEngine(nil, registry, nil, auditStore, 5*time.Minute, 0)
}

func TestWritePreview_EmptyBody(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/preview",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "at least one operation")
}

func TestWritePreview_InvalidJSON(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/preview",
		strings.NewReader(`{not valid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWriteDryRun_EmptyBody(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/dry-run",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWriteGetPlan_NotFound(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/write/plans/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestWriteApply_MissingToken(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/plans/some-id/apply",
		strings.NewReader(`{"actor":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "apply_token is required")
}

func TestWriteApply_PlanNotFound(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/plans/nonexistent/apply",
		strings.NewReader(`{"apply_token":"fake","actor":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "not found")
}

func TestWriteCancelPlan_NotFound(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/write/plans/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestWriteRollback_NotImplemented(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/rollback",
		strings.NewReader(`{"snapshot_id":1,"actor":"admin","reason":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "not yet implemented")
}

func TestWriteAuditLog_Empty(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/write/audit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var entries []any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entries))
	assert.Empty(t, entries)
}

func TestWriteAuditLog_InvalidLimit(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/write/audit?limit=abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWriteGetAuditEntry_NotFound(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/write/audit/999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestWriteGetAuditEntry_InvalidID(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/write/audit/abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWritePreview_OversizedBody(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	// Create a body larger than maxWriteBodySize (1 MB)
	big := strings.Repeat("x", 2<<20)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/preview",
		strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
