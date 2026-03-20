package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/correlate"
	"github.com/b42labs/northwatch/internal/enrich"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type correlatedTestData struct {
	switchUUID   string
	portUUID     string
	routerUUID   string
	lrpUUID      string
	chassisUUID  string
	datapathUUID string
	pbUUID       string
}

func setupCorrelatedTest(t *testing.T) (*http.ServeMux, correlatedTestData) {
	t.Helper()
	ctx := context.Background()

	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	// NB: Create LSP + LogicalSwitch
	namedLSPUUID := "lsp_corr"
	lsp := &nb.LogicalSwitchPort{
		UUID:        namedLSPUUID,
		Name:        "corr-port",
		ExternalIDs: map[string]string{"neutron:port_name": "my-port"},
	}
	lspOps, err := nbClient.Create(lsp)
	require.NoError(t, err)

	ls := &nb.LogicalSwitch{
		Name:        "corr-switch",
		Ports:       []string{namedLSPUUID},
		ExternalIDs: map[string]string{"neutron:network_name": "my-network"},
	}
	lsOps, err := nbClient.Create(ls)
	require.NoError(t, err)

	nbOps := append(lspOps, lsOps...)
	nbReply, err := nbClient.Transact(ctx, nbOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(nbReply, nbOps)
	require.NoError(t, err)

	portUUID := nbReply[0].UUID.GoUUID
	switchUUID := nbReply[1].UUID.GoUUID

	// NB: Create LRP + LogicalRouter
	namedLRPUUID := "lrp_corr"
	lrp := &nb.LogicalRouterPort{
		UUID:     namedLRPUUID,
		Name:     "corr-router-port",
		MAC:      "00:00:00:00:00:01",
		Networks: []string{"10.0.0.1/24"},
	}
	lrpOps, err := nbClient.Create(lrp)
	require.NoError(t, err)

	lr := &nb.LogicalRouter{
		Name:  "corr-router",
		Ports: []string{namedLRPUUID},
	}
	lrOps, err := nbClient.Create(lr)
	require.NoError(t, err)

	nbOps2 := append(lrpOps, lrOps...)
	nbReply2, err := nbClient.Transact(ctx, nbOps2...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(nbReply2, nbOps2)
	require.NoError(t, err)

	lrpUUID := nbReply2[0].UUID.GoUUID
	routerUUID := nbReply2[1].UUID.GoUUID

	require.Eventually(t, func() bool {
		sw := &nb.LogicalSwitch{UUID: switchUUID}
		rt := &nb.LogicalRouter{UUID: routerUUID}
		return nbClient.Get(ctx, sw) == nil && nbClient.Get(ctx, rt) == nil
	}, 2*time.Second, 10*time.Millisecond)

	// SB: Create Encap + Chassis
	namedEncapUUID := "encap_corr"
	encap := &sb.Encap{
		UUID:        namedEncapUUID,
		Type:        "geneve",
		IP:          "10.0.0.1",
		ChassisName: "corr-chassis",
	}
	encapOps, err := sbClient.Create(encap)
	require.NoError(t, err)

	chassis := &sb.Chassis{
		Name:     "corr-chassis",
		Hostname: "corr-host",
		Encaps:   []string{namedEncapUUID},
	}
	chassisOps, err := sbClient.Create(chassis)
	require.NoError(t, err)

	sbOps := append(encapOps, chassisOps...)
	sbReply, err := sbClient.Transact(ctx, sbOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(sbReply, sbOps)
	require.NoError(t, err)

	chassisUUID := sbReply[1].UUID.GoUUID

	require.Eventually(t, func() bool {
		ch := &sb.Chassis{UUID: chassisUUID}
		return sbClient.Get(ctx, ch) == nil
	}, 2*time.Second, 10*time.Millisecond)

	// SB: Create DatapathBinding + PortBinding
	namedDPUUID := "dp_corr"
	dp := &sb.DatapathBinding{
		UUID:        namedDPUUID,
		TunnelKey:   1,
		ExternalIDs: map[string]string{"logical-switch": switchUUID},
	}
	dpOps, err := sbClient.Create(dp)
	require.NoError(t, err)

	pb := &sb.PortBinding{
		LogicalPort: "corr-port",
		Datapath:    namedDPUUID,
		TunnelKey:   1,
		Chassis:     &chassisUUID,
	}
	pbOps, err := sbClient.Create(pb)
	require.NoError(t, err)

	sbOps2 := append(dpOps, pbOps...)
	sbReply2, err := sbClient.Transact(ctx, sbOps2...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(sbReply2, sbOps2)
	require.NoError(t, err)

	datapathUUID := sbReply2[0].UUID.GoUUID
	pbUUID := sbReply2[1].UUID.GoUUID

	require.Eventually(t, func() bool {
		d := &sb.DatapathBinding{UUID: datapathUUID}
		p := &sb.PortBinding{UUID: pbUUID}
		return sbClient.Get(ctx, d) == nil && sbClient.Get(ctx, p) == nil
	}, 2*time.Second, 10*time.Millisecond)

	cor := &correlate.Correlator{NB: nbClient, SB: sbClient}
	enricher := enrich.NewEnricher(nil, 0) // no-op enricher

	mux := http.NewServeMux()
	RegisterCorrelated(mux, cor, enricher)

	return mux, correlatedTestData{
		switchUUID:   switchUUID,
		portUUID:     portUUID,
		routerUUID:   routerUUID,
		lrpUUID:      lrpUUID,
		chassisUUID:  chassisUUID,
		datapathUUID: datapathUUID,
		pbUUID:       pbUUID,
	}
}

func TestCorrelated(t *testing.T) {
	mux, td := setupCorrelatedTest(t)

	t.Run("ListSwitches", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/correlated/logical-switches", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		ls := body[0]["logical_switch"].(map[string]any)
		assert.Equal(t, "corr-switch", ls["name"])
		assert.NotNil(t, body[0]["datapath_binding"])
	})

	t.Run("GetSwitch", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/correlated/logical-switches/%s", td.switchUUID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

		ls := body["logical_switch"].(map[string]any)
		assert.Equal(t, "corr-switch", ls["name"])
		assert.NotNil(t, body["datapath_binding"])

		ports, ok := body["ports"].([]any)
		require.True(t, ok)
		require.Len(t, ports, 1)
	})

	t.Run("GetSwitch_NotFound", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/correlated/logical-switches/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("ListRouters", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/correlated/logical-routers", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
	})

	t.Run("GetRouter", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/correlated/logical-routers/%s", td.routerUUID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		lr := body["logical_router"].(map[string]any)
		assert.Equal(t, "corr-router", lr["name"])
	})

	t.Run("GetLSP", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/correlated/logical-switch-ports/%s", td.portUUID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

		lsp := body["logical_switch_port"].(map[string]any)
		assert.Equal(t, "corr-port", lsp["name"])
		assert.NotNil(t, body["port_binding"])
		assert.NotNil(t, body["logical_switch"])
	})

	t.Run("GetLRP", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/correlated/logical-router-ports/%s", td.lrpUUID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

		lrp := body["logical_router_port"].(map[string]any)
		assert.Equal(t, "corr-router-port", lrp["name"])
		assert.NotNil(t, body["logical_router"])
	})

	t.Run("ListChassis", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/correlated/chassis", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		ch := body[0]["chassis"].(map[string]any)
		assert.Equal(t, "corr-chassis", ch["name"])
	})

	t.Run("GetChassis", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/correlated/chassis/%s", td.chassisUUID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		ch := body["chassis"].(map[string]any)
		assert.Equal(t, "corr-chassis", ch["name"])
		assert.NotNil(t, body["encaps"])
		assert.NotNil(t, body["port_bindings"])
	})

	t.Run("GetPortBinding", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/correlated/port-bindings/%s", td.pbUUID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

		pb := body["port_binding"].(map[string]any)
		assert.Equal(t, "corr-port", pb["logical_port"])
		assert.NotNil(t, body["logical_switch_port"])
		assert.NotNil(t, body["chassis"])
		assert.NotNil(t, body["logical_switch"])
	})
}

// mockProvider implements enrich.Provider for testing.
type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) EnrichPort(_ context.Context, externalIDs map[string]string) (*enrich.Info, error) {
	name := externalIDs["neutron:port_name"]
	if name == "" {
		return nil, nil
	}
	return &enrich.Info{DisplayName: name}, nil
}
func (m *mockProvider) EnrichNetwork(_ context.Context, externalIDs map[string]string) (*enrich.Info, error) {
	name := externalIDs["neutron:network_name"]
	if name == "" {
		return nil, nil
	}
	return &enrich.Info{DisplayName: name}, nil
}
func (m *mockProvider) EnrichRouter(_ context.Context, _ map[string]string) (*enrich.Info, error) {
	return nil, nil
}
func (m *mockProvider) EnrichNAT(_ context.Context, _ map[string]string) (*enrich.Info, error) {
	return nil, nil
}

func TestCorrelated_WithEnrichment(t *testing.T) {
	ctx := context.Background()

	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)

	// NB: Create LSP + LogicalSwitch with enrichable external_ids
	namedLSPUUID := "lsp_enrich"
	lsp := &nb.LogicalSwitchPort{
		UUID:        namedLSPUUID,
		Name:        "enrich-port",
		ExternalIDs: map[string]string{"neutron:port_name": "enriched-port"},
	}
	lspOps, err := nbClient.Create(lsp)
	require.NoError(t, err)

	ls := &nb.LogicalSwitch{
		Name:        "enrich-switch",
		Ports:       []string{namedLSPUUID},
		ExternalIDs: map[string]string{"neutron:network_name": "enriched-network"},
	}
	lsOps, err := nbClient.Create(ls)
	require.NoError(t, err)

	ops := append(lspOps, lsOps...)
	reply, err := nbClient.Transact(ctx, ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)

	switchUUID := reply[1].UUID.GoUUID

	// SB: Create DatapathBinding + PortBinding
	namedDPUUID := "dp_enrich"
	dp := &sb.DatapathBinding{
		UUID:        namedDPUUID,
		TunnelKey:   1,
		ExternalIDs: map[string]string{"logical-switch": switchUUID},
	}
	dpOps, err := sbClient.Create(dp)
	require.NoError(t, err)

	pb := &sb.PortBinding{
		LogicalPort: "enrich-port",
		Datapath:    namedDPUUID,
		TunnelKey:   1,
	}
	pbOps, err := sbClient.Create(pb)
	require.NoError(t, err)

	sbOps := append(dpOps, pbOps...)
	sbReply, err := sbClient.Transact(ctx, sbOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(sbReply, sbOps)
	require.NoError(t, err)

	datapathUUID := sbReply[0].UUID.GoUUID

	require.Eventually(t, func() bool {
		sw := &nb.LogicalSwitch{UUID: switchUUID}
		d := &sb.DatapathBinding{UUID: datapathUUID}
		return nbClient.Get(ctx, sw) == nil && sbClient.Get(ctx, d) == nil
	}, 2*time.Second, 10*time.Millisecond)

	cor := &correlate.Correlator{NB: nbClient, SB: sbClient}
	enricher := enrich.NewEnricher(&mockProvider{}, 5*time.Minute)

	mux := http.NewServeMux()
	RegisterCorrelated(mux, cor, enricher)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/api/v1/correlated/logical-switches/%s", switchUUID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// Check that enrichment is present on the switch
	ls2 := body["logical_switch"].(map[string]any)
	enrichment, ok := ls2["enrichment"].(map[string]any)
	require.True(t, ok, "switch should have enrichment")
	assert.Equal(t, "enriched-network", enrichment["display_name"])

	// Check that enrichment is present on the port
	ports := body["ports"].([]any)
	require.Len(t, ports, 1)
	port := ports[0].(map[string]any)
	lspMap := port["logical_switch_port"].(map[string]any)
	portEnrichment, ok := lspMap["enrichment"].(map[string]any)
	require.True(t, ok, "port should have enrichment")
	assert.Equal(t, "enriched-port", portEnrichment["display_name"])
}
