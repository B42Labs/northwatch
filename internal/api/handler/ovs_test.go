package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/b42labs/northwatch/internal/testutil"
)

// setupOVS builds a pool with one connected chassis ("chassis-1") seeded with a
// br-int bridge and a vnet0 interface, and returns the registered mux.
func setupOVS(t *testing.T) *http.ServeMux {
	t.Helper()
	sock, seed := testutil.SetupOVSTestServer(t)
	testutil.InsertOVSBridgeWithInterface(t, seed, "br-int", "vnet0", map[string]int{"rx_packets": 7, "tx_packets": 9}, "up")

	return ovsMuxForSock(t, sock, func(c client.Client) bool {
		var ifaces []vs.Interface
		if err := c.List(context.Background(), &ifaces); err != nil {
			return false
		}
		return len(ifaces) == 1
	})
}

// ovsMuxForSock dials a "chassis-1" pool at the given OVS socket, waits until it
// is connected and the ready predicate holds against its monitored cache, then
// returns the registered mux. It isolates the pool wiring shared by the seeded
// fixtures.
func ovsMuxForSock(t *testing.T, sock string, ready func(client.Client) bool) *http.ServeMux {
	t.Helper()
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Add("chassis-1", "unix:"+sock))

	c, ok := pool.Client("chassis-1")
	require.True(t, ok)
	require.Eventually(t, c.Connected, 5*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return ready(c) }, 5*time.Second, 20*time.Millisecond)

	mux := http.NewServeMux()
	RegisterOVS(mux, pool)
	return mux
}

func ovsGet(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestOVSListInterface(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface")
	require.Equal(t, http.StatusOK, w.Code)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	// The live telemetry fields the SB DB cannot provide must be present.
	assert.Contains(t, rows[0], "statistics")
	assert.Contains(t, rows[0], "link_state")
	assert.Contains(t, rows[0], "error")
	assert.Equal(t, "vnet0", rows[0]["name"])
}

func TestOVSGetInterface(t *testing.T) {
	mux := setupOVS(t)
	list := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	uuid, _ := rows[0]["_uuid"].(string)
	require.NotEmpty(t, uuid)

	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface/"+uuid)
	require.Equal(t, http.StatusOK, w.Code)
	var row map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &row))
	assert.Equal(t, "vnet0", row["name"])
}

func TestOVSListBridge(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/bridge")
	require.Equal(t, http.StatusOK, w.Code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "br-int", rows[0]["name"])
	assert.Contains(t, rows[0], "datapath_type")
}

func TestOVSFleet(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs")
	require.Equal(t, http.StatusOK, w.Code)
	var members []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &members))
	require.Len(t, members, 1)
	assert.Equal(t, "chassis-1", members[0]["system_id"])
	assert.Equal(t, true, members[0]["connected"])
}

func TestOVSUnknownChassis(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/nope/interface")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOVSUnknownTable(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/bogus")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOVSRowNotFound(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface/00000000-0000-0000-0000-000000000000")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOVSNewTablesRoutable(t *testing.T) {
	mux := setupOVS(t)
	// The tables widened beyond the original interface/bridge/port/open-vswitch/
	// manager/controller set: each must be routable (200 + JSON array), not 404.
	slugs := []string{
		"ipfix", "sflow", "netflow", "mirror", "qos", "queue",
		"ct-zone", "ct-timeout-policy", "datapath",
		"flow-table", "flow-sample-collector-set", "ssl", "autoattach",
	}
	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/"+slug)
			require.Equal(t, http.StatusOK, w.Code)
			var rows []map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
		})
	}
}

func TestOVSSSLRedactsPrivateKey(t *testing.T) {
	sock, seed := testutil.SetupOVSTestServer(t)
	testutil.InsertOVSSSLRow(t, seed, "SECRET-PRIVATE-KEY", "PUBLIC-CERT", "CA-CERT")

	mux := ovsMuxForSock(t, sock, func(c client.Client) bool {
		var rows []vs.SSL
		if err := c.List(context.Background(), &rows); err != nil {
			return false
		}
		return len(rows) == 1
	})

	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/ssl")
	require.Equal(t, http.StatusOK, w.Code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	// The sensitive key material must never reach the client, under any key;
	// public certs stay.
	assert.NotContains(t, w.Body.String(), "SECRET-PRIVATE-KEY")
	assert.NotContains(t, rows[0], "private_key")
	assert.Equal(t, "PUBLIC-CERT", rows[0]["certificate"])
	assert.Equal(t, "CA-CERT", rows[0]["ca_cert"])
}

func TestOVSUnreachable(t *testing.T) {
	// A registered-but-unreachable chassis serves 503, not 404 or a cache read.
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Add("down", "unix:/nonexistent/northwatch-ovs.sock"))

	c, ok := pool.Client("down")
	require.True(t, ok)
	require.False(t, c.Connected())

	mux := http.NewServeMux()
	RegisterOVS(mux, pool)
	w := ovsGet(t, mux, "/api/v1/ovs/down/interface")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
