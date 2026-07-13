package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/history"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/b42labs/northwatch/internal/write"

	_ "modernc.org/sqlite"
)

func setupTestWriteEngine(t *testing.T) *write.Engine {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	auditStore, err := write.NewAuditStore(context.Background(), db)
	require.NoError(t, err)

	registry := write.DefaultRegistry()
	// Pass nil nbClient and nil collector — tests that need them will fail gracefully.
	engine, err := write.NewEngine(nil, nil, registry, nil, auditStore, 5*time.Minute, 0)
	require.NoError(t, err)
	return engine
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

func TestWriteRollback_NoCollector(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/rollback",
		strings.NewReader(`{"snapshot_id":1,"actor":"admin","reason":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// A missing history collector is a server-capability failure, not a bad
	// request: it maps to 500 under the corrected status taxonomy.
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "requires history collector")
}

func TestWriteEngineErrorTaxonomy(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"rate limited", write.ErrRateLimited, http.StatusTooManyRequests},
		{"stale state", write.ErrStaleState, http.StatusConflict},
		{"wrapped stale state", fmt.Errorf("transact: %w", write.ErrStaleState), http.StatusConflict},
		{"input error", &write.InputError{Message: "bad field"}, http.StatusBadRequest},
		{"infra error", errors.New("cache list failed"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeEngineError(w, tc.err)
			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func TestApplyEngineErrorTaxonomy(t *testing.T) {
	entry := &write.AuditEntry{}
	tests := []struct {
		name  string
		entry *write.AuditEntry
		err   error
		want  int
	}{
		{"rate limited", nil, write.ErrRateLimited, http.StatusTooManyRequests},
		{"stale state", entry, fmt.Errorf("transact: %w", write.ErrStaleState), http.StatusConflict},
		// A fresh input error at apply (preview validated clean) means the
		// referenced state changed since preview: a 409, never a 500.
		{"stale reference at apply", entry,
			fmt.Errorf("operation 0: %w", &write.InputError{Message: "referenced Load_Balancer does not exist"}),
			http.StatusConflict},
		{"bad plan (no entry)", nil, errors.New("plan \"x\" not found or expired"), http.StatusBadRequest},
		{"infra failure", entry, errors.New("transact: connection refused"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			applyEngineError(w, tc.entry, tc.err)
			assert.Equal(t, tc.want, w.Code)
		})
	}
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

func TestWriteSchema(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/write/schema", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=3600")
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body, "tables")
}

func TestWriteAuditLog_ValidLimit(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/write/audit?limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var entries []any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entries))
	assert.Empty(t, entries)
}

func TestWriteRollback_InvalidJSON(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/rollback",
		strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWriteRollback_MissingSnapshotID(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/rollback",
		strings.NewReader(`{"actor":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "snapshot_id is required")
}

// TestWritePreview_InvalidOperation drives an operation that fails structural
// validation (unknown table) before the engine touches the NB client, so it
// exercises decodeWriteRequest's reason-propagation branch and handlePreview's
// error path with the nil-client test engine.
func TestWritePreview_InvalidOperation(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	body := `{"reason":"cleanup","operations":[{"action":"update","table":"No_Such_Table","uuid":"x"}]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/preview",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWriteDryRun_InvalidOperation(t *testing.T) {
	engine := setupTestWriteEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	body := `{"reason":"cleanup","operations":[{"action":"update","table":"No_Such_Table","uuid":"x"}]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/dry-run",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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

// setupApplyEngine builds a write engine with a real NB client and history
// collector, so a plan can be previewed and applied end-to-end and the resulting
// audit entry inspected.
func setupApplyEngine(t *testing.T) *write.Engine {
	t.Helper()
	nbClient := testutil.SetupNBTestClient(t)

	historyStore, err := history.NewStore(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = historyStore.Close() })

	sources := []history.TableSource{{
		Database: "nb",
		Table:    "Logical_Switch",
		ListFunc: func(ctx context.Context) ([]map[string]any, error) {
			var switches []nb.LogicalSwitch
			if err := nbClient.List(ctx, &switches); err != nil {
				return nil, err
			}
			return ovndb.ModelsToMaps(switches), nil
		},
	}}
	collector := history.NewCollector(historyStore, nil, sources, time.Hour, time.Hour)

	auditStore, err := write.NewAuditStore(context.Background(), historyStore.DB())
	require.NoError(t, err)
	engine, err := write.NewEngine(nbClient, nil, write.DefaultRegistry(), collector, auditStore, 5*time.Minute, 0)
	require.NoError(t, err)
	return engine
}

// authorized returns ctx carrying the actor the auth middleware would attach for
// the named token.
func authorized(name string) context.Context {
	tokens := map[string]string{name: "0123456789abcdef"}
	var got context.Context
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Context() })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/write/preview", nil)
	req.Header.Set("Authorization", "Bearer 0123456789abcdef")
	api.AuthMiddleware(tokens)(next).ServeHTTP(httptest.NewRecorder(), req)
	return got
}

// TestWriteApply_ActorDerivedFromToken drives preview → apply with a client-sent
// "actor" field and asserts the audit entry names the authenticated token
// instead: a client can no longer forge who made a change.
func TestWriteApply_ActorDerivedFromToken(t *testing.T) {
	engine := setupApplyEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	ctx := authorized("ops")

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/write/preview",
		strings.NewReader(`{"operations":[{"action":"create","table":"Logical_Switch","fields":{"name":"ls-actor"}}],"reason":"test"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var plan struct {
		ID         string `json:"id"`
		ApplyToken string `json:"apply_token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))

	req = httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/write/plans/"+plan.ID+"/apply",
		strings.NewReader(`{"apply_token":"`+plan.ApplyToken+`","actor":"attacker"}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var entry write.AuditEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
	assert.Equal(t, "ops", entry.Actor)

	entries, err := engine.QueryAudit(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "ops", entries[0].Actor)
}

// TestWriteApply_ActorAnonymousWithoutCredential covers the --insecure-no-auth
// deployment, where no credential reaches the handler: the audit entry must be
// attributed to "anonymous" rather than to a client-supplied string.
func TestWriteApply_ActorAnonymousWithoutCredential(t *testing.T) {
	engine := setupApplyEngine(t)
	mux := http.NewServeMux()
	RegisterWrite(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/preview",
		strings.NewReader(`{"operations":[{"action":"create","table":"Logical_Switch","fields":{"name":"ls-anon"}}],"reason":"test"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var plan struct {
		ID         string `json:"id"`
		ApplyToken string `json:"apply_token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))

	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/write/plans/"+plan.ID+"/apply",
		strings.NewReader(`{"apply_token":"`+plan.ApplyToken+`","actor":"admin"}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var entry write.AuditEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
	assert.Equal(t, "anonymous", entry.Actor)
}
