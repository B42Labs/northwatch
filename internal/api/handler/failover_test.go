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

	"github.com/b42labs/northwatch/internal/write"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func setupFailoverMux(t *testing.T) *http.ServeMux {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	auditStore, err := write.NewAuditStore(db)
	require.NoError(t, err)

	// nil nbClient is fine — validation tests never reach the engine's OVSDB calls.
	engine := write.NewEngine(nil, nil, write.DefaultRegistry(), nil, auditStore, 5*time.Minute, 0)

	mux := http.NewServeMux()
	RegisterFailover(mux, engine)
	return mux
}

func TestFailoverHandler_MissingGroupName(t *testing.T) {
	mux := setupFailoverMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/failover",
		strings.NewReader(`{"target_chassis": "chassis-2"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "group_name")
}

func TestFailoverHandler_MissingTargetChassis(t *testing.T) {
	mux := setupFailoverMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/failover",
		strings.NewReader(`{"group_name": "test-group"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "target_chassis")
}

func TestEvacuateHandler_MissingChassisName(t *testing.T) {
	mux := setupFailoverMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/evacuate",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "chassis_name")
}

func TestFailoverHandler_InvalidJSON(t *testing.T) {
	mux := setupFailoverMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/failover",
		strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestEvacuateHandler_InvalidJSON(t *testing.T) {
	mux := setupFailoverMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/evacuate",
		strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
