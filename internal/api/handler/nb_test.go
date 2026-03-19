package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupNBTestClient(t *testing.T) client.Client {
	t.Helper()

	clientModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	schema := nb.Schema()

	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)

	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{
		schema.Name: clientModel,
	}, &logger)

	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)

	sockPath := filepath.Join(t.TempDir(), "nb.sock")
	go func() {
		_ = ovsdbServer.Serve("unix", sockPath)
	}()

	require.Eventually(t, func() bool {
		return ovsdbServer.Ready()
	}, 5*time.Second, 10*time.Millisecond)

	t.Cleanup(func() {
		ovsdbServer.Close()
	})

	c, err := client.NewOVSDBClient(
		clientModel,
		client.WithEndpoint(fmt.Sprintf("unix:%s", sockPath)),
	)
	require.NoError(t, err)

	err = c.Connect(context.Background())
	require.NoError(t, err)

	_, err = c.MonitorAll(context.Background())
	require.NoError(t, err)

	t.Cleanup(func() {
		c.Close()
	})

	return c
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
