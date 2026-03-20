package debug

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectivityChecker(t *testing.T) {
	nbClient := setupNBClient(t)
	sbClient := setupSBClient(t)
	ctx := context.Background()

	checker := &ConnectivityChecker{NB: nbClient, SB: sbClient}

	chassisUUID, datapathUUID := createSBInfra(t, ctx, sbClient)

	t.Run("SameSwitchBothBound", func(t *testing.T) {
		// Create two ports on the same switch
		namedLSP1 := "lsp_conn_1"
		lsp1 := &nb.LogicalSwitchPort{
			UUID:      namedLSP1,
			Name:      "port-a",
			Addresses: []string{"00:00:00:00:00:0a 10.0.0.10"},
		}
		lsp1Ops, _ := nbClient.Create(lsp1)

		namedLSP2 := "lsp_conn_2"
		lsp2 := &nb.LogicalSwitchPort{
			UUID:      namedLSP2,
			Name:      "port-b",
			Addresses: []string{"00:00:00:00:00:0b 10.0.0.11"},
		}
		lsp2Ops, _ := nbClient.Create(lsp2)

		ls := &nb.LogicalSwitch{
			Name:  "switch-conn",
			Ports: []string{namedLSP1, namedLSP2},
		}
		lsOps, _ := nbClient.Create(ls)

		nbOps := append(lsp1Ops, lsp2Ops...)
		nbOps = append(nbOps, lsOps...)
		nbReply := transact(t, ctx, nbClient, nbOps...)
		lsp1UUID := nbReply[0].UUID.GoUUID
		lsp2UUID := nbReply[1].UUID.GoUUID

		// Create port bindings using real datapath UUID
		pb1 := &sb.PortBinding{
			LogicalPort: "port-a",
			Datapath:    datapathUUID,
			TunnelKey:   20,
			Chassis:     &chassisUUID,
		}
		pb1Ops, _ := sbClient.Create(pb1)

		pb2 := &sb.PortBinding{
			LogicalPort: "port-b",
			Datapath:    datapathUUID,
			TunnelKey:   21,
			Chassis:     &chassisUUID,
		}
		pb2Ops, _ := sbClient.Create(pb2)

		sbOps := append(pb1Ops, pb2Ops...)
		transact(t, ctx, sbClient, sbOps...)

		require.Eventually(t, func() bool {
			l := &nb.LogicalSwitchPort{UUID: lsp1UUID}
			return nbClient.Get(ctx, l) == nil
		}, 2*time.Second, 10*time.Millisecond)

		result, err := checker.Check(ctx, lsp1UUID, lsp2UUID)
		require.NoError(t, err)

		assert.Equal(t, "port-a", result.Source.Name)
		assert.Equal(t, "port-b", result.Destination.Name)

		// All resolution checks should pass
		for _, check := range result.Checks {
			if check.Category == "resolution" {
				assert.Equal(t, StatusPass, check.Status, "resolution check %s", check.Name)
			}
		}
	})

	t.Run("SameSwitchOneUnbound", func(t *testing.T) {
		namedLSP3 := "lsp_conn_3"
		lsp3 := &nb.LogicalSwitchPort{
			UUID:      namedLSP3,
			Name:      "port-c",
			Addresses: []string{"00:00:00:00:00:0c 10.0.0.12"},
		}
		lsp3Ops, _ := nbClient.Create(lsp3)

		namedLSP4 := "lsp_conn_4"
		lsp4 := &nb.LogicalSwitchPort{
			UUID:      namedLSP4,
			Name:      "port-d-unbound",
			Addresses: []string{"00:00:00:00:00:0d 10.0.0.13"},
		}
		lsp4Ops, _ := nbClient.Create(lsp4)

		ls := &nb.LogicalSwitch{
			Name:  "switch-conn-2",
			Ports: []string{namedLSP3, namedLSP4},
		}
		lsOps, _ := nbClient.Create(ls)

		nbOps := append(lsp3Ops, lsp4Ops...)
		nbOps = append(nbOps, lsOps...)
		nbReply := transact(t, ctx, nbClient, nbOps...)
		lsp3UUID := nbReply[0].UUID.GoUUID
		lsp4UUID := nbReply[1].UUID.GoUUID

		// Only bind port-c
		pb3 := &sb.PortBinding{
			LogicalPort: "port-c",
			Datapath:    datapathUUID,
			TunnelKey:   30,
			Chassis:     &chassisUUID,
		}
		pb3Ops, _ := sbClient.Create(pb3)
		transact(t, ctx, sbClient, pb3Ops...)

		require.Eventually(t, func() bool {
			l := &nb.LogicalSwitchPort{UUID: lsp3UUID}
			return nbClient.Get(ctx, l) == nil
		}, 2*time.Second, 10*time.Millisecond)

		result, err := checker.Check(ctx, lsp3UUID, lsp4UUID)
		require.NoError(t, err)

		// Should have a physical fail for the unbound port
		var hasFail bool
		for _, check := range result.Checks {
			if check.Category == "physical" && check.Status == StatusFail {
				hasFail = true
			}
		}
		assert.True(t, hasFail, "expected physical fail for unbound port")
	})

	t.Run("NonExistentPorts", func(t *testing.T) {
		result, err := checker.Check(ctx, "nonexistent-1", "nonexistent-2")
		require.NoError(t, err)
		assert.Equal(t, StatusFail, result.Overall)
	})
}
