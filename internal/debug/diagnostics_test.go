package debug

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

func transact(t *testing.T, ctx context.Context, c client.Client, ops ...ovsdb.Operation) []ovsdb.OperationResult {
	t.Helper()
	reply, err := c.Transact(ctx, ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	return reply
}

// createSBInfra creates a Chassis + DatapathBinding and returns their real UUIDs.
func createSBInfra(t *testing.T, ctx context.Context, sbClient client.Client) (chassisUUID, datapathUUID string) {
	t.Helper()

	// Create Encap + Chassis
	namedEncapUUID := "encap_infra"
	encap := &sb.Encap{
		UUID:        namedEncapUUID,
		Type:        "geneve",
		IP:          "10.0.0.1",
		ChassisName: "chassis-1",
	}
	encapOps, err := sbClient.Create(encap)
	require.NoError(t, err)

	chassis := &sb.Chassis{
		Name:     "chassis-1",
		Hostname: "host-1",
		Encaps:   []string{namedEncapUUID},
	}
	chassisOps, err := sbClient.Create(chassis)
	require.NoError(t, err)

	sbOps := append(encapOps, chassisOps...)
	sbReply := transact(t, ctx, sbClient, sbOps...)
	chassisUUID = sbReply[1].UUID.GoUUID

	require.Eventually(t, func() bool {
		ch := &sb.Chassis{UUID: chassisUUID}
		return sbClient.Get(ctx, ch) == nil
	}, 2*time.Second, 10*time.Millisecond)

	// Create DatapathBinding
	dp := &sb.DatapathBinding{
		TunnelKey: 1,
		ExternalIDs: map[string]string{
			"logical-switch": "sw-uuid",
		},
	}
	dpOps, err := sbClient.Create(dp)
	require.NoError(t, err)
	dpReply := transact(t, ctx, sbClient, dpOps...)
	datapathUUID = dpReply[0].UUID.GoUUID

	require.Eventually(t, func() bool {
		d := &sb.DatapathBinding{UUID: datapathUUID}
		return sbClient.Get(ctx, d) == nil
	}, 2*time.Second, 10*time.Millisecond)

	return chassisUUID, datapathUUID
}

func TestPortDiagnoser(t *testing.T) {
	nbClient := setupNBClient(t)
	sbClient := setupSBClient(t)
	ctx := context.Background()

	diagnoser := &PortDiagnoser{NB: nbClient, SB: sbClient}

	chassisUUID, datapathUUID := createSBInfra(t, ctx, sbClient)

	t.Run("HealthyVIFPort", func(t *testing.T) {
		up := true
		enabled := true
		namedLSPUUID := "lsp_healthy"
		lsp := &nb.LogicalSwitchPort{
			UUID:      namedLSPUUID,
			Name:      "healthy-port",
			Type:      "",
			Addresses: []string{"00:00:00:00:00:01 10.0.0.2"},
			Up:        &up,
			Enabled:   &enabled,
		}
		lspOps, err := nbClient.Create(lsp)
		require.NoError(t, err)

		ls := &nb.LogicalSwitch{
			Name:  "test-switch",
			Ports: []string{namedLSPUUID},
		}
		lsOps, err := nbClient.Create(ls)
		require.NoError(t, err)

		nbOps := append(lspOps, lsOps...)
		nbReply := transact(t, ctx, nbClient, nbOps...)
		lspUUID := nbReply[0].UUID.GoUUID

		// Create port binding using real datapath UUID
		pb := &sb.PortBinding{
			LogicalPort: "healthy-port",
			Datapath:    datapathUUID,
			TunnelKey:   10,
			Chassis:     &chassisUUID,
			Type:        "",
		}
		pbOps, err := sbClient.Create(pb)
		require.NoError(t, err)
		transact(t, ctx, sbClient, pbOps...)

		require.Eventually(t, func() bool {
			l := &nb.LogicalSwitchPort{UUID: lspUUID}
			return nbClient.Get(ctx, l) == nil
		}, 2*time.Second, 10*time.Millisecond)

		diag, err := diagnoser.DiagnosePort(ctx, lspUUID)
		require.NoError(t, err)
		assert.Equal(t, SeverityHealthy, diag.Overall)
		assert.Equal(t, "healthy-port", diag.PortName)
		assert.Equal(t, "test-switch", diag.SwitchName)

		for _, check := range diag.Checks {
			assert.Equal(t, SeverityHealthy, check.Status, "check %s should be healthy", check.Name)
		}
	})

	t.Run("UnboundVIFPort", func(t *testing.T) {
		namedLSPUUID := "lsp_unbound"
		lsp := &nb.LogicalSwitchPort{
			UUID:      namedLSPUUID,
			Name:      "unbound-port",
			Type:      "",
			Addresses: []string{"00:00:00:00:00:02 10.0.0.3"},
		}
		lspOps, err := nbClient.Create(lsp)
		require.NoError(t, err)

		ls := &nb.LogicalSwitch{
			Name:  "unbound-switch",
			Ports: []string{namedLSPUUID},
		}
		lsOps, err := nbClient.Create(ls)
		require.NoError(t, err)

		nbOps := append(lspOps, lsOps...)
		nbReply := transact(t, ctx, nbClient, nbOps...)
		lspUUID := nbReply[0].UUID.GoUUID

		require.Eventually(t, func() bool {
			l := &nb.LogicalSwitchPort{UUID: lspUUID}
			return nbClient.Get(ctx, l) == nil
		}, 2*time.Second, 10*time.Millisecond)

		diag, err := diagnoser.DiagnosePort(ctx, lspUUID)
		require.NoError(t, err)
		assert.Equal(t, SeverityError, diag.Overall)

		var bindingCheck *DiagnosticCheck
		for _, check := range diag.Checks {
			if check.Name == "binding_status" {
				bindingCheck = &check
				break
			}
		}
		require.NotNil(t, bindingCheck)
		assert.Equal(t, SeverityError, bindingCheck.Status)
	})

	t.Run("DisabledPort", func(t *testing.T) {
		enabled := false
		namedLSPUUID := "lsp_disabled"
		lsp := &nb.LogicalSwitchPort{
			UUID:      namedLSPUUID,
			Name:      "disabled-port",
			Type:      "",
			Addresses: []string{"00:00:00:00:00:03 10.0.0.4"},
			Enabled:   &enabled,
		}
		lspOps, err := nbClient.Create(lsp)
		require.NoError(t, err)

		ls := &nb.LogicalSwitch{
			Name:  "disabled-switch",
			Ports: []string{namedLSPUUID},
		}
		lsOps, err := nbClient.Create(ls)
		require.NoError(t, err)

		nbOps := append(lspOps, lsOps...)
		nbReply := transact(t, ctx, nbClient, nbOps...)
		lspUUID := nbReply[0].UUID.GoUUID

		require.Eventually(t, func() bool {
			l := &nb.LogicalSwitchPort{UUID: lspUUID}
			return nbClient.Get(ctx, l) == nil
		}, 2*time.Second, 10*time.Millisecond)

		diag, err := diagnoser.DiagnosePort(ctx, lspUUID)
		require.NoError(t, err)
		assert.NotEqual(t, SeverityHealthy, diag.Overall)

		var stateCheck *DiagnosticCheck
		for _, check := range diag.Checks {
			if check.Name == "port_state" {
				stateCheck = &check
				break
			}
		}
		require.NotNil(t, stateCheck)
		assert.Equal(t, SeverityWarning, stateCheck.Status)
	})

	t.Run("RouterPortMissingPeer", func(t *testing.T) {
		namedLSPUUID := "lsp_router_nopeer"
		lsp := &nb.LogicalSwitchPort{
			UUID: namedLSPUUID,
			Name: "router-port-nopeer",
			Type: "router",
			Options: map[string]string{
				"router-port": "nonexistent-lrp",
			},
		}
		lspOps, err := nbClient.Create(lsp)
		require.NoError(t, err)

		ls := &nb.LogicalSwitch{
			Name:  "router-nopeer-switch",
			Ports: []string{namedLSPUUID},
		}
		lsOps, err := nbClient.Create(ls)
		require.NoError(t, err)

		nbOps := append(lspOps, lsOps...)
		nbReply := transact(t, ctx, nbClient, nbOps...)
		lspUUID := nbReply[0].UUID.GoUUID

		require.Eventually(t, func() bool {
			l := &nb.LogicalSwitchPort{UUID: lspUUID}
			return nbClient.Get(ctx, l) == nil
		}, 2*time.Second, 10*time.Millisecond)

		diag, err := diagnoser.DiagnosePort(ctx, lspUUID)
		require.NoError(t, err)
		assert.Equal(t, SeverityError, diag.Overall)

		var routerCheck *DiagnosticCheck
		for _, check := range diag.Checks {
			if check.Name == "router_peer" {
				routerCheck = &check
				break
			}
		}
		require.NotNil(t, routerCheck)
		assert.Equal(t, SeverityError, routerCheck.Status)
	})

	t.Run("DiagnoseAll", func(t *testing.T) {
		summary, err := diagnoser.DiagnoseAll(ctx)
		require.NoError(t, err)
		assert.Greater(t, summary.Total, 0)
		assert.Equal(t, summary.Total, summary.Healthy+summary.Warning+summary.Error)

		// Errors should come first
		if len(summary.Ports) >= 2 {
			assert.True(t, severityOrder(summary.Ports[0].Overall) <= severityOrder(summary.Ports[len(summary.Ports)-1].Overall))
		}
	})
}
