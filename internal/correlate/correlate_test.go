package correlate

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupNBClient(t *testing.T) client.Client {
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

func setupSBClient(t *testing.T) client.Client {
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

// testData holds correlated test entities across NB and SB.
type testData struct {
	SwitchUUID   string
	PortUUID     string
	RouterUUID   string
	LRPortUUID   string
	NATUUID      string
	DatapathUUID string
	PBPortUUID   string
	ChassisUUID  string
	EncapUUID    string
}

func insertCorrelatedData(t *testing.T, nbClient, sbClient client.Client) testData {
	t.Helper()
	ctx := context.Background()

	// NB: Create LSP + LogicalSwitch
	namedLSPUUID := "lsp_test_port"
	lsp := &nb.LogicalSwitchPort{
		UUID:        namedLSPUUID,
		Name:        "test-port",
		ExternalIDs: map[string]string{"neutron:port_name": "my-port"},
	}
	lspOps, err := nbClient.Create(lsp)
	require.NoError(t, err)

	ls := &nb.LogicalSwitch{
		Name:        "test-switch",
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

	// NB: Create LRP + NAT + LogicalRouter
	namedLRPUUID := "lrp_test_port"
	lrp := &nb.LogicalRouterPort{
		UUID:     namedLRPUUID,
		Name:     "test-router-port",
		MAC:      "00:00:00:00:00:01",
		Networks: []string{"10.0.0.1/24"},
	}
	lrpOps, err := nbClient.Create(lrp)
	require.NoError(t, err)

	namedNATUUID := "nat_test"
	nat := &nb.NAT{
		UUID:       namedNATUUID,
		Type:       "dnat_and_snat",
		ExternalIP: "192.168.1.100",
		LogicalIP:  "10.0.0.2",
	}
	natOps, err := nbClient.Create(nat)
	require.NoError(t, err)

	lr := &nb.LogicalRouter{
		Name:        "test-router",
		Ports:       []string{namedLRPUUID},
		Nat:         []string{namedNATUUID},
		ExternalIDs: map[string]string{"neutron:router_name": "my-router"},
	}
	lrOps, err := nbClient.Create(lr)
	require.NoError(t, err)

	nbOps2 := append(lrpOps, natOps...)
	nbOps2 = append(nbOps2, lrOps...)
	nbReply2, err := nbClient.Transact(ctx, nbOps2...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(nbReply2, nbOps2)
	require.NoError(t, err)

	lrpUUID := nbReply2[0].UUID.GoUUID
	natUUID := nbReply2[1].UUID.GoUUID
	routerUUID := nbReply2[2].UUID.GoUUID

	// Wait for NB cache
	require.Eventually(t, func() bool {
		sw := &nb.LogicalSwitch{UUID: switchUUID}
		rt := &nb.LogicalRouter{UUID: routerUUID}
		return nbClient.Get(ctx, sw) == nil && nbClient.Get(ctx, rt) == nil
	}, 2*time.Second, 10*time.Millisecond)

	// SB: Create Encap + Chassis
	namedEncapUUID := "encap_test"
	encap := &sb.Encap{
		UUID:        namedEncapUUID,
		Type:        "geneve",
		IP:          "192.168.1.1",
		ChassisName: "test-chassis",
	}
	encapOps, err := sbClient.Create(encap)
	require.NoError(t, err)

	chassis := &sb.Chassis{
		Name:     "test-chassis",
		Hostname: "host-1",
		Encaps:   []string{namedEncapUUID},
	}
	chassisOps, err := sbClient.Create(chassis)
	require.NoError(t, err)

	sbOps := append(encapOps, chassisOps...)
	sbReply, err := sbClient.Transact(ctx, sbOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(sbReply, sbOps)
	require.NoError(t, err)

	encapUUID := sbReply[0].UUID.GoUUID
	chassisUUID := sbReply[1].UUID.GoUUID

	require.Eventually(t, func() bool {
		ch := &sb.Chassis{UUID: chassisUUID}
		return sbClient.Get(ctx, ch) == nil
	}, 2*time.Second, 10*time.Millisecond)

	// SB: Create DatapathBinding + PortBinding (for LSP)
	namedDPUUID := "dp_switch"
	dp := &sb.DatapathBinding{
		UUID:      namedDPUUID,
		TunnelKey: 1,
		ExternalIDs: map[string]string{
			"logical-switch": switchUUID,
		},
	}
	dpOps, err := sbClient.Create(dp)
	require.NoError(t, err)

	pb := &sb.PortBinding{
		LogicalPort: "test-port",
		Datapath:    namedDPUUID,
		TunnelKey:   1,
		Chassis:     &chassisUUID,
		ExternalIDs: map[string]string{"neutron:port_name": "my-port"},
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

	// SB: Create ChassisPrivate
	cp := &sb.ChassisPrivate{
		Name:    "test-chassis",
		Chassis: &chassisUUID,
	}
	cpOps, err := sbClient.Create(cp)
	require.NoError(t, err)
	cpReply, err := sbClient.Transact(ctx, cpOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(cpReply, cpOps)
	require.NoError(t, err)

	// Wait for SB cache
	require.Eventually(t, func() bool {
		d := &sb.DatapathBinding{UUID: datapathUUID}
		p := &sb.PortBinding{UUID: pbUUID}
		return sbClient.Get(ctx, d) == nil && sbClient.Get(ctx, p) == nil
	}, 2*time.Second, 10*time.Millisecond)

	return testData{
		SwitchUUID:   switchUUID,
		PortUUID:     portUUID,
		RouterUUID:   routerUUID,
		LRPortUUID:   lrpUUID,
		NATUUID:      natUUID,
		DatapathUUID: datapathUUID,
		PBPortUUID:   pbUUID,
		ChassisUUID:  chassisUUID,
		EncapUUID:    encapUUID,
	}
}

func TestCorrelate(t *testing.T) {
	nbClient := setupNBClient(t)
	sbClient := setupSBClient(t)
	td := insertCorrelatedData(t, nbClient, sbClient)

	cor := &Correlator{NB: nbClient, SB: sbClient}
	ctx := context.Background()

	t.Run("SwitchSummary", func(t *testing.T) {
		ls := &nb.LogicalSwitch{UUID: td.SwitchUUID}
		err := nbClient.Get(ctx, ls)
		require.NoError(t, err)

		result := cor.SwitchSummary(ctx, *ls)
		assert.Equal(t, "test-switch", result.Switch["name"])
		require.NotNil(t, result.Datapath)
		assert.Equal(t, td.SwitchUUID, result.Datapath["external_ids"].(map[string]string)["logical-switch"])
		assert.Nil(t, result.Ports)
	})

	t.Run("SwitchDetail", func(t *testing.T) {
		result, err := cor.SwitchDetail(ctx, td.SwitchUUID)
		require.NoError(t, err)

		assert.Equal(t, "test-switch", result.Switch["name"])
		require.NotNil(t, result.Datapath)
		require.Len(t, result.Ports, 1)

		port := result.Ports[0]
		assert.Equal(t, "test-port", port.LSP["name"])
		require.NotNil(t, port.PortBinding)
		assert.Equal(t, "test-port", port.PortBinding["logical_port"])
		require.NotNil(t, port.Chassis)
		assert.Equal(t, "test-chassis", port.Chassis["name"])
		require.NotNil(t, port.Datapath)
	})

	t.Run("SwitchDetail_NotFound", func(t *testing.T) {
		_, err := cor.SwitchDetail(ctx, "nonexistent")
		assert.Error(t, err)
	})

	t.Run("RouterDetail", func(t *testing.T) {
		result, err := cor.RouterDetail(ctx, td.RouterUUID)
		require.NoError(t, err)

		assert.Equal(t, "test-router", result.Router["name"])
		require.Len(t, result.Ports, 1)
		assert.Equal(t, "test-router-port", result.Ports[0].LRP["name"])
		require.Len(t, result.NATs, 1)
		assert.Equal(t, "dnat_and_snat", result.NATs[0]["type"])
		assert.Equal(t, "192.168.1.100", result.NATs[0]["external_ip"])
	})

	t.Run("LSPDetail", func(t *testing.T) {
		chain, err := cor.LSPDetail(ctx, td.PortUUID)
		require.NoError(t, err)

		assert.Equal(t, "test-port", chain.LSP["name"])
		require.NotNil(t, chain.PortBinding)
		require.NotNil(t, chain.Chassis)
		require.NotNil(t, chain.ParentSwitch)
		assert.Equal(t, "test-switch", chain.ParentSwitch["name"])
	})

	t.Run("LRPDetail", func(t *testing.T) {
		chain, err := cor.LRPDetail(ctx, td.LRPortUUID)
		require.NoError(t, err)

		assert.Equal(t, "test-router-port", chain.LRP["name"])
		require.NotNil(t, chain.ParentRouter)
		assert.Equal(t, "test-router", chain.ParentRouter["name"])
		// No PortBinding for this LRP since we didn't create one in SB
		assert.Nil(t, chain.PortBinding)
	})

	t.Run("ChassisDetail", func(t *testing.T) {
		result, err := cor.ChassisDetail(ctx, td.ChassisUUID)
		require.NoError(t, err)

		assert.Equal(t, "test-chassis", result.Chassis["name"])
		require.NotNil(t, result.ChassisPrivate)
		assert.Equal(t, "test-chassis", result.ChassisPrivate["name"])
		require.Len(t, result.Encaps, 1)
		assert.Equal(t, "192.168.1.1", result.Encaps[0]["ip"])
		require.Len(t, result.PortBindings, 1)
		assert.Equal(t, "test-port", result.PortBindings[0]["logical_port"])
	})

	t.Run("PortBindingDetail", func(t *testing.T) {
		chain, err := cor.PortBindingDetail(ctx, td.PBPortUUID)
		require.NoError(t, err)

		assert.Equal(t, "test-port", chain.PortBinding["logical_port"])
		require.NotNil(t, chain.LSP)
		assert.Equal(t, "test-port", chain.LSP["name"])
		require.NotNil(t, chain.Chassis)
		require.NotNil(t, chain.Datapath)
		require.NotNil(t, chain.ParentSwitch)
		assert.Equal(t, "test-switch", chain.ParentSwitch["name"])
	})

	t.Run("PartialChain_NoChassis", func(t *testing.T) {
		// Create LSP + parent switch in NB (LSP must be referenced to avoid GC)
		namedLSPUUID := "lsp_unbound"
		lsp := &nb.LogicalSwitchPort{
			UUID: namedLSPUUID,
			Name: "unbound-port",
		}
		lspOps, err := nbClient.Create(lsp)
		require.NoError(t, err)

		unboundSwitch := &nb.LogicalSwitch{
			Name:  "unbound-switch",
			Ports: []string{namedLSPUUID},
		}
		lsOps, err := nbClient.Create(unboundSwitch)
		require.NoError(t, err)

		nbOps := append(lspOps, lsOps...)
		nbReply, err := nbClient.Transact(ctx, nbOps...)
		require.NoError(t, err)
		_, err = ovsdb.CheckOperationResults(nbReply, nbOps)
		require.NoError(t, err)

		lspUUID := nbReply[0].UUID.GoUUID

		require.Eventually(t, func() bool {
			l := &nb.LogicalSwitchPort{UUID: lspUUID}
			return nbClient.Get(ctx, l) == nil
		}, 2*time.Second, 10*time.Millisecond)

		// Create port binding without chassis in SB
		namedDPUUID := "dp_nochassis"
		dp := &sb.DatapathBinding{
			UUID:      namedDPUUID,
			TunnelKey: 2,
			ExternalIDs: map[string]string{
				"logical-switch": td.SwitchUUID,
			},
		}
		dpOps, err := sbClient.Create(dp)
		require.NoError(t, err)

		pb := &sb.PortBinding{
			LogicalPort: "unbound-port",
			Datapath:    namedDPUUID,
			TunnelKey:   2,
		}
		pbOps, err := sbClient.Create(pb)
		require.NoError(t, err)

		sbOps := append(dpOps, pbOps...)
		sbReply, err := sbClient.Transact(ctx, sbOps...)
		require.NoError(t, err)
		_, err = ovsdb.CheckOperationResults(sbReply, sbOps)
		require.NoError(t, err)

		pbUUID := sbReply[1].UUID.GoUUID

		require.Eventually(t, func() bool {
			p := &sb.PortBinding{UUID: pbUUID}
			return sbClient.Get(ctx, p) == nil
		}, 2*time.Second, 10*time.Millisecond)

		chain, err := cor.PortBindingDetail(ctx, pbUUID)
		require.NoError(t, err)

		assert.Equal(t, "unbound-port", chain.PortBinding["logical_port"])
		require.NotNil(t, chain.LSP)
		assert.Nil(t, chain.Chassis, "chassis should be nil for unbound port")
		require.NotNil(t, chain.Datapath)
	})
}
