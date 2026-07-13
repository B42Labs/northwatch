package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupNBTestClient(t *testing.T) client.Client {
	t.Helper()
	return testutil.SetupNBTestClient(t)
}

func insertLogicalSwitch(t *testing.T, c client.Client, name string) string {
	t.Helper()

	ls := &nb.LogicalSwitch{
		Name:        name,
		ExternalIDs: map[string]string{"test": "true"},
	}
	ops, err := c.Create(ls)
	require.NoError(t, err)

	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)

	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)

	uuid := reply[0].UUID.GoUUID

	// Wait for cache to be updated
	require.Eventually(t, func() bool {
		sw := &nb.LogicalSwitch{UUID: uuid}
		return c.Get(context.Background(), sw) == nil
	}, 2*time.Second, 10*time.Millisecond)

	return uuid
}

func TestNBListLogicalSwitches(t *testing.T) {
	c := setupNBTestClient(t)
	insertLogicalSwitch(t, c, "test-switch-1")
	insertLogicalSwitch(t, c, "test-switch-2")

	mux := http.NewServeMux()
	RegisterNB(mux, c)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/nb/logical-switches", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body, 2)

	// Check that ovsdb tags are used as keys
	names := []string{}
	for _, item := range body {
		name, ok := item["name"].(string)
		require.True(t, ok)
		names = append(names, name)
	}
	assert.Contains(t, names, "test-switch-1")
	assert.Contains(t, names, "test-switch-2")
}

func TestNBGetLogicalSwitch(t *testing.T) {
	c := setupNBTestClient(t)
	uuid := insertLogicalSwitch(t, c, "my-switch")

	mux := http.NewServeMux()
	RegisterNB(mux, c)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/nb/logical-switches/%s", uuid), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "my-switch", body["name"])
	assert.Equal(t, uuid, body["_uuid"])
}

func TestNBGetLogicalSwitch_NotFound(t *testing.T) {
	c := setupNBTestClient(t)

	mux := http.NewServeMux()
	RegisterNB(mux, c)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/nb/logical-switches/nonexistent-uuid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestNBListLogicalSwitches_Empty(t *testing.T) {
	c := setupNBTestClient(t)

	mux := http.NewServeMux()
	RegisterNB(mux, c)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/nb/logical-switches", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Empty(t, body)
}

// TestHandleList_Pagination drives the limit/offset window on a table list. The
// cap matters most on the unbounded tables (SB logical flows), but every list
// endpoint shares this code path.
func TestHandleList_Pagination(t *testing.T) {
	nbc := setupNBTestClient(t)
	for i := range 12 {
		insertLogicalSwitch(t, nbc, fmt.Sprintf("sw-page-%02d", i))
	}

	mux := http.NewServeMux()
	RegisterNB(mux, nbc)

	list := func(t *testing.T, query string) (*httptest.ResponseRecorder, []map[string]any) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
			"/api/v1/nb/logical-switches"+query, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var got []map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		return rec, got
	}

	t.Run("no params returns everything under the cap", func(t *testing.T) {
		rec, got := list(t, "")
		assert.Len(t, got, 12)
		assert.Equal(t, "12", rec.Header().Get("X-Total-Count"))
		assert.Empty(t, rec.Header().Get("X-Truncated"))
	})

	t.Run("limit truncates and flags", func(t *testing.T) {
		rec, got := list(t, "?limit=5")
		assert.Len(t, got, 5)
		assert.Equal(t, "12", rec.Header().Get("X-Total-Count"))
		assert.Equal(t, "true", rec.Header().Get("X-Truncated"))
	})

	t.Run("offset windows deterministically", func(t *testing.T) {
		_, first := list(t, "?limit=4")
		_, second := list(t, "?limit=4&offset=4")
		_, whole := list(t, "")

		// A stable order is what makes offset meaningful: the cache iterates a
		// map, so without sorting a window could repeat a row and skip another.
		assert.Equal(t, whole[0:4], first)
		assert.Equal(t, whole[4:8], second)
	})

	t.Run("offset past the end yields an empty page", func(t *testing.T) {
		rec, got := list(t, "?offset=99")
		assert.Empty(t, got)
		assert.Equal(t, "12", rec.Header().Get("X-Total-Count"))
		assert.Empty(t, rec.Header().Get("X-Truncated"))
	})

	t.Run("limit above the cap is clamped, not rejected", func(t *testing.T) {
		rec, got := list(t, fmt.Sprintf("?limit=%d", maxListPageSize*10))
		assert.Len(t, got, 12)
		assert.Equal(t, "12", rec.Header().Get("X-Total-Count"))
		assert.Empty(t, rec.Header().Get("X-Truncated"))
	})
}

func TestHandleList_InvalidPageParams(t *testing.T) {
	nbc := setupNBTestClient(t)
	mux := http.NewServeMux()
	RegisterNB(mux, nbc)

	for _, query := range []string{"?limit=abc", "?limit=-1", "?offset=-1", "?offset=abc"} {
		t.Run(query, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
				"/api/v1/nb/logical-switches"+query, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "non-negative integer")
		})
	}
}
