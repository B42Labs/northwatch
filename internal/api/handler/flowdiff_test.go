package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/flowdiff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleFlowDiff_Empty(t *testing.T) {
	store := flowdiff.NewStore(100, time.Hour)
	handler := handleFlowDiff(store)

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/debug/flow-diff", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp FlowDiffResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Count)
	assert.Empty(t, resp.Changes)
}

func TestHandleFlowDiff_WithChanges(t *testing.T) {
	store := flowdiff.NewStore(100, time.Hour)
	now := time.Now().UnixMilli()
	store.Add(flowdiff.FlowChange{
		Timestamp: now - 2000,
		Type:      "insert",
		UUID:      "flow-1",
		Datapath:  "dp-1",
		NewRow:    map[string]any{"match": "ip4", "actions": "next;"},
	})
	store.Add(flowdiff.FlowChange{
		Timestamp: now - 1000,
		Type:      "delete",
		UUID:      "flow-2",
		Datapath:  "dp-1",
		OldRow:    map[string]any{"match": "ip6", "actions": "drop;"},
	})

	handler := handleFlowDiff(store)

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/debug/flow-diff?datapath=dp-1", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp FlowDiffResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Count)
	assert.Equal(t, "insert", resp.Changes[0].Type)
	assert.Equal(t, "delete", resp.Changes[1].Type)
}

func TestHandleFlowDiff_SinceFilter(t *testing.T) {
	store := flowdiff.NewStore(100, time.Hour)
	now := time.Now().UnixMilli()
	store.Add(flowdiff.FlowChange{
		Timestamp: now - 5000,
		Type:      "insert",
		UUID:      "old",
		Datapath:  "dp",
	})
	store.Add(flowdiff.FlowChange{
		Timestamp: now - 1000,
		Type:      "insert",
		UUID:      "new",
		Datapath:  "dp",
	})

	handler := handleFlowDiff(store)

	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("/api/v1/debug/flow-diff?since=%d", now-3000), nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp FlowDiffResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Count)
	assert.Equal(t, "new", resp.Changes[0].UUID)
}

func TestHandleFlowDiff_InvalidSince(t *testing.T) {
	store := flowdiff.NewStore(100, time.Hour)
	handler := handleFlowDiff(store)

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/debug/flow-diff?since=invalid", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
