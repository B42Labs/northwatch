package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/testutil"
)

// TestInventoryHandler exercises the aggregated chassis-inventory routes on a
// single shared SB server. It also asserts that the inventory routes coexist on
// one mux with the raw Chassis table route (`/api/v1/sb/chassis`) — Go 1.22's
// ServeMux panics on conflicting patterns at registration time, so registering
// both without a panic is itself the conflict check.
func TestInventoryHandler(t *testing.T) {
	c := setupSBTestClient(t)

	testutil.InsertSBGlobal(t, c, 5)
	chUUID := insertChassis(t, c, "node-a", "node-a.example", "10.0.0.1")
	testutil.InsertChassisPrivate(t, c, "node-a", &chUUID, 5, int(time.Now().UnixMilli()))
	testutil.InsertPortBinding(t, c, "vif-1", "", &chUUID)
	insertChassis(t, c, "node-b", "node-b.example", "10.0.0.2")

	mux := http.NewServeMux()
	RegisterSB(mux, c) // raw Chassis table at /api/v1/sb/chassis
	RegisterInventory(mux, c, 60*time.Second)

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/chassis-inventory", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body, 2)
		assert.Equal(t, "node-a", body[0]["name"])
	})

	t.Run("detail", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/chassis-inventory/node-a", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "node-a", body["name"])

		liveness, ok := body["liveness"].(map[string]any)
		require.True(t, ok, "liveness object missing")
		assert.Equal(t, true, liveness["in_sync"])

		boundPorts, ok := body["bound_ports"].([]any)
		require.True(t, ok, "bound_ports array missing")
		assert.Len(t, boundPorts, 1)
	})

	t.Run("detail not found", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/chassis-inventory/ghost", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("raw chassis route still works", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/chassis", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body, 2)
	})
}
