package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHistorySetup(t *testing.T) (*history.Store, *history.Collector, *http.ServeMux) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := history.NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	hub := events.NewHub()
	sources := []history.TableSource{
		{
			Database: "nb",
			Table:    "Logical_Switch",
			ListFunc: func(ctx context.Context) ([]map[string]any, error) {
				return []map[string]any{
					{"_uuid": "uuid-1", "name": "sw1"},
				}, nil
			},
		},
	}
	collector := history.NewCollector(store, hub, sources, 1*time.Hour, 24*time.Hour)

	mux := http.NewServeMux()
	RegisterHistory(mux, store, collector)
	return store, collector, mux
}

func TestHistory_ListSnapshots_Empty(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result []history.SnapshotMeta
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Empty(t, result)
}

func TestHistory_CreateAndListSnapshots(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	// Create snapshot
	req := httptest.NewRequestWithContext(context.Background(),"POST", "/api/v1/snapshots", strings.NewReader(`{"label":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var created history.SnapshotMeta
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.Equal(t, "manual", created.Trigger)
	assert.Equal(t, "test", created.Label)
	assert.Equal(t, 1, created.RowCounts["nb.Logical_Switch"])

	// List
	req = httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var list []history.SnapshotMeta
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list))
	assert.Len(t, list, 1)
}

func TestHistory_GetSnapshot(t *testing.T) {
	store, _, mux := newTestHistorySetup(t)
	ctx := context.Background()

	rows := []history.SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	meta, err := store.CreateSnapshot(ctx, "manual", "test", rows)
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got history.SnapshotMeta
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, meta.ID, got.ID)
}

func TestHistory_GetSnapshot_NotFound(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHistory_GetSnapshotRows(t *testing.T) {
	store, _, mux := newTestHistorySetup(t)
	ctx := context.Background()

	rows := []history.SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
		{Database: "sb", Table: "Chassis", UUID: "uuid-2", Data: map[string]any{"hostname": "node1"}},
	}
	_, err := store.CreateSnapshot(ctx, "manual", "", rows)
	require.NoError(t, err)

	// All rows
	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/1/rows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got []history.SnapshotRow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Len(t, got, 2)

	// Filtered
	req = httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/1/rows?database=nb", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Len(t, got, 1)
}

func TestHistory_DeleteSnapshot(t *testing.T) {
	store, _, mux := newTestHistorySetup(t)
	ctx := context.Background()

	_, err := store.CreateSnapshot(ctx, "manual", "", nil)
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(),"DELETE", "/api/v1/snapshots/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify gone
	req = httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/1", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHistory_DeleteSnapshot_NotFound(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"DELETE", "/api/v1/snapshots/999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHistory_DiffSnapshots(t *testing.T) {
	store, _, mux := newTestHistorySetup(t)
	ctx := context.Background()

	rows1 := []history.SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1"}},
	}
	rows2 := []history.SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: "uuid-1", Data: map[string]any{"name": "sw1-changed"}},
	}
	_, err := store.CreateSnapshot(ctx, "manual", "", rows1)
	require.NoError(t, err)
	_, err = store.CreateSnapshot(ctx, "manual", "", rows2)
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/diff?from=1&to=2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var diff history.DiffResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&diff))
	assert.Len(t, diff.Tables, 1)
	assert.Len(t, diff.Tables[0].Modified, 1)
}

func TestHistory_DiffSnapshots_MissingParams(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/diff", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHistory_QueryEvents(t *testing.T) {
	store, _, mux := newTestHistorySetup(t)
	ctx := context.Background()

	err := store.InsertEvent(ctx, history.EventRecord{
		Timestamp: time.Now().UTC(),
		Type:      "insert",
		Database:  "nb",
		Table:     "Logical_Switch",
		UUID:      "uuid-1",
		Row:       map[string]any{"name": "sw1"},
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/events", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got []history.EventRecord
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Len(t, got, 1)
}

func TestHistory_QueryEvents_WithFilters(t *testing.T) {
	store, _, mux := newTestHistorySetup(t)
	ctx := context.Background()

	err := store.InsertEvents(ctx, []history.EventRecord{
		{Timestamp: time.Now().UTC(), Type: "insert", Database: "nb", Table: "Logical_Switch", UUID: "uuid-1"},
		{Timestamp: time.Now().UTC(), Type: "delete", Database: "sb", Table: "Chassis", UUID: "uuid-2"},
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/events?database=nb&type=insert", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got []history.EventRecord
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Len(t, got, 1)
	assert.Equal(t, "nb", got[0].Database)
}

func TestHistory_InvalidSnapshotID(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHistory_CreateSnapshot_NoBody(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"POST", "/api/v1/snapshots", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var created history.SnapshotMeta
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.Equal(t, "manual", created.Trigger)
	assert.Empty(t, created.Label)
}

func TestHistory_DiffSnapshots_InvalidFrom(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/snapshots/diff?from=abc&to=2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHistory_QueryEvents_InvalidLimit(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/events?limit=abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHistory_QueryEvents_InvalidSince(t *testing.T) {
	_, _, mux := newTestHistorySetup(t)

	req := httptest.NewRequestWithContext(context.Background(),"GET", "/api/v1/events?since=not-a-date", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
