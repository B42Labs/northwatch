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

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSBTestClient(t *testing.T) client.Client {
	t.Helper()

	clientModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)
	schema := sb.Schema()

	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)

	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{
		schema.Name: clientModel,
	}, &logger)

	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)

	sockPath := filepath.Join(t.TempDir(), "sb.sock")
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

func insertChassis(t *testing.T, c client.Client, name, hostname, ip string) string {
	t.Helper()

	namedEncapUUID := "encap_" + name
	encap := &sb.Encap{
		UUID:        namedEncapUUID,
		Type:        "geneve",
		IP:          ip,
		ChassisName: name,
	}
	encapOps, err := c.Create(encap)
	require.NoError(t, err)

	chassis := &sb.Chassis{
		Name:     name,
		Hostname: hostname,
		Encaps:   []string{namedEncapUUID},
	}
	chassisOps, err := c.Create(chassis)
	require.NoError(t, err)

	ops := append(encapOps, chassisOps...)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)

	uuid := reply[1].UUID.GoUUID

	require.Eventually(t, func() bool {
		ch := &sb.Chassis{UUID: uuid}
		return c.Get(context.Background(), ch) == nil
	}, 2*time.Second, 10*time.Millisecond)

	return uuid
}

// TestSB runs all SB handler tests under a single shared test server
// to avoid file descriptor exhaustion from multiple servers.
func TestSB(t *testing.T) {
	c := setupSBTestClient(t)

	t.Run("ListChassis", func(t *testing.T) {
		insertChassis(t, c, "chassis-1", "host-1", "192.168.1.1")
		insertChassis(t, c, "chassis-2", "host-2", "192.168.1.2")

		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/chassis", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body, 2)
	})

	t.Run("GetChassis", func(t *testing.T) {
		// chassis-1 was inserted by previous subtest
		var chassisList []sb.Chassis
		err := c.List(context.Background(), &chassisList)
		require.NoError(t, err)
		require.NotEmpty(t, chassisList)

		uuid := chassisList[0].UUID

		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/sb/chassis/%s", uuid), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotEmpty(t, body["name"])
	})

	t.Run("LogicalFlows_Empty", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/logical-flows", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Empty(t, body)
	})

	t.Run("LogicalFlows_InvalidTableID", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/logical-flows?table_id=abc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandleLogicalFlows_Filters drives the filtered branch of
// handleLogicalFlows against a small seeded flow set on the shared SB server.
func TestHandleLogicalFlows_Filters(t *testing.T) {
	sbc := testutil.SetupSBTestClient(t)

	dp := insertDatapath(t, sbc, map[string]string{"logical-switch": "sw-flt"})
	insertLogicalFlow(t, sbc, dp, "ingress", 0, 100, `inport == "a"`, "next;", nil)
	insertLogicalFlow(t, sbc, dp, "ingress", 3, 50, `ip4.dst == 10.0.0.1`, "next;", nil)
	insertLogicalFlow(t, sbc, dp, "egress", 0, 10, "1", "output;", nil)

	mux := http.NewServeMux()
	RegisterSB(mux, sbc)

	get := func(t *testing.T, url string) []map[string]any {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body
	}

	t.Run("by pipeline and datapath", func(t *testing.T) {
		body := get(t, "/api/v1/sb/logical-flows?datapath="+dp+"&pipeline=ingress")
		assert.Len(t, body, 2)
	})

	t.Run("by table_id", func(t *testing.T) {
		body := get(t, "/api/v1/sb/logical-flows?datapath="+dp+"&table_id=3")
		assert.Len(t, body, 1)
	})

	t.Run("by match substring", func(t *testing.T) {
		body := get(t, "/api/v1/sb/logical-flows?match=ip4.dst")
		assert.Len(t, body, 1)
	})
}
