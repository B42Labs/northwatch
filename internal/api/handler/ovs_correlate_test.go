package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/b42labs/northwatch/internal/testutil"
)

// correlationResult mirrors the JSON shape of ovscorrelate.Correlation.
type correlationResult struct {
	IfaceID string `json:"iface_id"`
	Bound   bool   `json:"bound"`
	Binding *struct {
		LogicalPort string `json:"logical_port"`
		Up          *bool  `json:"up"`
		Chassis     string `json:"chassis"`
		BoundHere   bool   `json:"bound_here"`
		Datapath    string `json:"datapath"`
	} `json:"binding"`
	Drift []string `json:"drift"`
}

// TestOVSInterfaceCorrelation exercises the correlation endpoint end-to-end
// against one in-memory OVS server (seeded with several live interfaces) and one
// Southbound client. A single shared fixture keeps the server/client footprint
// small; each scenario is a subtest resolving a different seeded interface.
func TestOVSInterfaceCorrelation(t *testing.T) {
	sock, seed := testutil.SetupOVSTestServer(t)
	// Four live interfaces, each under its own bridge:
	//   vnet-up      iface-id lsp-a,      link up   → bound, no drift
	//   vnet-down    iface-id lsp-a,      link down → bound, drift
	//   vnet-orphan  iface-id lsp-orphan, link up   → iface-id but no SB binding
	//   vnet-noid    no iface-id,         link up   → not OVN-managed
	testutil.InsertOVSInterfaceWithIfaceID(t, seed, "br-up", "vnet-up", "lsp-a", "up")
	testutil.InsertOVSInterfaceWithIfaceID(t, seed, "br-down", "vnet-down", "lsp-a", "down")
	testutil.InsertOVSInterfaceWithIfaceID(t, seed, "br-orphan", "vnet-orphan", "lsp-orphan", "up")
	testutil.InsertOVSBridgeWithInterface(t, seed, "br-noid", "vnet-noid", map[string]int{}, "up")

	sbClient := testutil.SetupSBTestClient(t)
	chassisUUID := testutil.InsertChassis(t, sbClient, "chassis-1", "host-1", "10.0.0.1")
	up := true
	testutil.InsertPortBindingWithUp(t, sbClient, "lsp-a", "", &chassisUUID, &up)

	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Add("chassis-1", "unix:"+sock))

	c, ok := pool.Client("chassis-1")
	require.True(t, ok)
	require.Eventually(t, c.Connected, 5*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool {
		var ifaces []vs.Interface
		if err := c.List(context.Background(), &ifaces); err != nil {
			return false
		}
		return len(ifaces) == 4
	}, 5*time.Second, 20*time.Millisecond)

	mux := http.NewServeMux()
	RegisterOVS(mux, pool)
	RegisterOVSCorrelation(mux, pool, sbClient)

	// uuidByName resolves a seeded interface name to its cache UUID via the list
	// route, so the correlation route can be addressed by UUID.
	uuidByName := func(name string) string {
		list := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface")
		require.Equal(t, http.StatusOK, list.Code)
		var rows []map[string]any
		require.NoError(t, json.Unmarshal(list.Body.Bytes(), &rows))
		for _, row := range rows {
			if row["name"] == name {
				uuid, _ := row["_uuid"].(string)
				require.NotEmpty(t, uuid)
				return uuid
			}
		}
		t.Fatalf("interface %q not found in cache", name)
		return ""
	}

	getCorrelation := func(path string) correlationResult {
		w := ovsGet(t, mux, path)
		require.Equal(t, http.StatusOK, w.Code)
		var result correlationResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		return result
	}

	t.Run("bound and up has no drift", func(t *testing.T) {
		uuid := uuidByName("vnet-up")
		result := getCorrelation("/api/v1/ovs/chassis-1/interface/" + uuid + "/correlation")
		assert.Equal(t, "lsp-a", result.IfaceID)
		assert.True(t, result.Bound)
		require.NotNil(t, result.Binding)
		assert.Equal(t, "lsp-a", result.Binding.LogicalPort)
		require.NotNil(t, result.Binding.Up)
		assert.True(t, *result.Binding.Up)
		assert.True(t, result.Binding.BoundHere)
		assert.Equal(t, "chassis-1", result.Binding.Chassis)
		assert.NotEmpty(t, result.Binding.Datapath)
		assert.Empty(t, result.Drift)
	})

	t.Run("SB up but interface down is drift", func(t *testing.T) {
		uuid := uuidByName("vnet-down")
		result := getCorrelation("/api/v1/ovs/chassis-1/interface/" + uuid + "/correlation")
		require.True(t, result.Bound)
		assert.NotEmpty(t, result.Drift)
	})

	t.Run("iface-id without an SB binding degrades to unbound", func(t *testing.T) {
		uuid := uuidByName("vnet-orphan")
		result := getCorrelation("/api/v1/ovs/chassis-1/interface/" + uuid + "/correlation")
		assert.Equal(t, "lsp-orphan", result.IfaceID)
		assert.False(t, result.Bound)
		assert.Nil(t, result.Binding)
	})

	t.Run("no iface-id is not OVN-managed", func(t *testing.T) {
		uuid := uuidByName("vnet-noid")
		result := getCorrelation("/api/v1/ovs/chassis-1/interface/" + uuid + "/correlation")
		assert.Empty(t, result.IfaceID)
		assert.False(t, result.Bound)
		assert.Nil(t, result.Binding)
	})

	t.Run("unknown chassis is 404", func(t *testing.T) {
		uuid := uuidByName("vnet-up")
		w := ovsGet(t, mux, "/api/v1/ovs/nope/interface/"+uuid+"/correlation")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unknown interface uuid is 404", func(t *testing.T) {
		w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface/00000000-0000-0000-0000-000000000000/correlation")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
