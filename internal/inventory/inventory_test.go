package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/inventory"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
)

// t0 is a fixed wall-clock used as Builder.Now so liveness ("alive") is
// deterministic. Timestamps in the fixtures are expressed relative to it.
const t0 = int64(1_700_000_000_000)

func fixedNow() time.Time { return time.UnixMilli(t0) }

func boolPtr(b bool) *bool { return &b }

// insertChassisWithConfig inserts a Chassis (plus its required geneve Encap)
// carrying the given other_config copies. testutil.InsertChassis always sets an
// empty other_config, so the inventory's bridge-mappings / datapath-type /
// iface-types / cms-options parsing needs this richer helper.
func insertChassisWithConfig(t *testing.T, c client.Client, name, hostname, ip string, otherConfig map[string]string) string {
	t.Helper()
	encapUUID := "encap_" + name
	encap := &sb.Encap{UUID: encapUUID, Type: "geneve", IP: ip, ChassisName: name}
	encapOps, err := c.Create(encap)
	require.NoError(t, err)
	ch := &sb.Chassis{
		Name: name, Hostname: hostname, Encaps: []string{encapUUID},
		ExternalIDs: map[string]string{}, OtherConfig: otherConfig,
	}
	chOps, err := c.Create(ch)
	require.NoError(t, err)
	ops := append(encapOps, chOps...)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[1].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.Chassis{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// TestBuilderList exercises the aggregated list against a single SB fixture
// covering identity/system-id, tunnel endpoints, bridge mappings, the bound-port
// summary, and the three liveness shapes (in-sync+alive, out-of-sync+stale, and
// no Chassis_Private heartbeat at all).
func TestBuilderList(t *testing.T) {
	c := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, c, 5)

	// Insert out of alphabetical order to prove List sorts by name. gamma-id
	// deliberately has no Chassis_Private row, so its UUID is not needed.
	testutil.InsertChassis(t, c, "gamma-id", "gamma-host", "10.0.0.3")
	deltaUUID := insertChassisWithConfig(t, c, "delta-id", "delta-host", "10.0.0.4", map[string]string{
		"ovn-bridge-mappings": "physnet1:br-ex,physnet2:br-ex2",
		"datapath-type":       "system",
		"iface-types":         "geneve,vxlan",
		"ovn-cms-options":     "enable-chassis-as-gw",
	})
	betaUUID := testutil.InsertChassis(t, c, "beta-id", "beta-host", "10.0.0.2")
	alphaUUID := testutil.InsertChassis(t, c, "alpha-id", "alpha-host", "10.0.0.1")

	// Liveness fixtures.
	testutil.InsertChassisPrivate(t, c, "alpha-id", &alphaUUID, 5, int(t0-1000))  // in-sync, fresh
	testutil.InsertChassisPrivate(t, c, "beta-id", &betaUUID, 3, int(t0-120_000)) // behind, stale
	testutil.InsertChassisPrivate(t, c, "delta-id", &deltaUUID, 5, int(t0-500))   // in-sync, fresh
	// gamma-id intentionally has no Chassis_Private row.

	// Bound ports on alpha: mixed type and up state.
	testutil.InsertPortBindingWithUp(t, c, "vif-1", "", &alphaUUID, boolPtr(true))
	testutil.InsertPortBindingWithUp(t, c, "vif-2", "", &alphaUUID, boolPtr(false))
	testutil.InsertPortBinding(t, c, "patch-1", "patch", &alphaUUID) // Up unset
	// An unbound port must never be attributed to any chassis.
	testutil.InsertPortBinding(t, c, "unbound-1", "", nil)

	b := &inventory.Builder{SB: c, StaleThreshold: 60 * time.Second, Now: fixedNow}
	list, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 4)

	// Sorted by name.
	gotOrder := []string{list[0].Name, list[1].Name, list[2].Name, list[3].Name}
	assert.Equal(t, []string{"alpha-id", "beta-id", "delta-id", "gamma-id"}, gotOrder)

	byName := map[string]inventory.ChassisSummary{}
	for _, s := range list {
		byName[s.Name] = s
	}

	tests := []struct {
		name               string
		wantHostname       string
		wantEncaps         []inventory.EncapInfo
		wantBridgeMappings string
		wantInSync         bool
		wantAlive          bool
		wantNbCfg          int
		wantPorts          inventory.PortSummary
	}{
		{
			name:         "alpha-id",
			wantHostname: "alpha-host",
			wantEncaps:   []inventory.EncapInfo{{Type: "geneve", IP: "10.0.0.1"}},
			wantInSync:   true,
			wantAlive:    true,
			wantNbCfg:    5,
			wantPorts:    inventory.PortSummary{Total: 3, Up: 1, ByType: map[string]int{"": 2, "patch": 1}},
		},
		{
			name:         "beta-id",
			wantHostname: "beta-host",
			wantEncaps:   []inventory.EncapInfo{{Type: "geneve", IP: "10.0.0.2"}},
			wantInSync:   false, // nb_cfg 3 != sb_global 5
			wantAlive:    false, // 120s old, past the 60s threshold
			wantNbCfg:    3,
			wantPorts:    inventory.PortSummary{Total: 0, Up: 0, ByType: map[string]int{}},
		},
		{
			name:               "delta-id",
			wantHostname:       "delta-host",
			wantEncaps:         []inventory.EncapInfo{{Type: "geneve", IP: "10.0.0.4"}},
			wantBridgeMappings: "physnet1:br-ex,physnet2:br-ex2",
			wantInSync:         true,
			wantAlive:          true,
			wantNbCfg:          5,
			wantPorts:          inventory.PortSummary{Total: 0, Up: 0, ByType: map[string]int{}},
		},
		{
			name:         "gamma-id",
			wantHostname: "gamma-host",
			wantEncaps:   []inventory.EncapInfo{{Type: "geneve", IP: "10.0.0.3"}},
			wantInSync:   false, // no Chassis_Private heartbeat
			wantAlive:    false,
			wantNbCfg:    0,
			wantPorts:    inventory.PortSummary{Total: 0, Up: 0, ByType: map[string]int{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, ok := byName[tc.name]
			require.True(t, ok, "chassis %q missing from list", tc.name)

			// name is the identity (== OVS system-id) and the Phase 2 join key.
			assert.Equal(t, tc.name, s.Name)
			assert.Equal(t, tc.wantHostname, s.Hostname)
			assert.Equal(t, tc.wantEncaps, s.Encaps)
			assert.Equal(t, tc.wantBridgeMappings, s.BridgeMappings)

			assert.Equal(t, tc.wantInSync, s.Liveness.InSync)
			assert.Equal(t, tc.wantAlive, s.Liveness.Alive)
			assert.Equal(t, tc.wantNbCfg, s.Liveness.NbCfg)
			assert.Equal(t, 5, s.Liveness.SBNbCfg)

			assert.Equal(t, tc.wantPorts, s.Ports)
		})
	}

	t.Run("alive age is computed against the fixed clock", func(t *testing.T) {
		alpha := byName["alpha-id"]
		assert.Equal(t, int64(t0-1000), alpha.Liveness.NbCfgTimestamp)
		assert.Equal(t, int64(1000), alpha.Liveness.AgeMs)
	})
}

// TestBuilderFutureTimestamp guards against a future nb_cfg_timestamp (a fast or
// misbehaving chassis clock) pinning alive=true and leaking a negative age_ms:
// nb_cfg_timestamp is written by the chassis's own ovn-controller, so it must
// not be trusted to push the chassis past the staleness check.
func TestBuilderFutureTimestamp(t *testing.T) {
	c := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, c, 5)
	uuid := testutil.InsertChassis(t, c, "skewed-id", "skewed-host", "10.0.0.9")
	// nb_cfg_timestamp 10s ahead of the fixed host clock.
	testutil.InsertChassisPrivate(t, c, "skewed-id", &uuid, 5, int(t0+10_000))

	b := &inventory.Builder{SB: c, StaleThreshold: 60 * time.Second, Now: fixedNow}
	list, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)

	lv := list[0].Liveness
	assert.False(t, lv.Alive, "a future nb_cfg_timestamp must not report alive")
	assert.Equal(t, int64(0), lv.AgeMs, "a future timestamp must not leak a negative age")
	assert.Equal(t, int64(t0+10_000), lv.NbCfgTimestamp)
}

// TestBuilderListCachesWithinTTL pins the short-TTL result cache: a chassis
// added within the window stays invisible until the TTL elapses, proving
// repeated requests reuse one computed snapshot instead of re-scanning the SB
// tables on every call.
func TestBuilderListCachesWithinTTL(t *testing.T) {
	c := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, c, 1)
	testutil.InsertChassis(t, c, "node-1", "host-1", "10.0.0.1")

	nowMs := t0
	b := &inventory.Builder{
		SB:             c,
		StaleThreshold: 60 * time.Second,
		Now:            func() time.Time { return time.UnixMilli(nowMs) },
	}

	first, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, first, 1)

	// A second chassis added within the TTL is not yet visible: the cached
	// snapshot is served.
	testutil.InsertChassis(t, c, "node-2", "host-2", "10.0.0.2")
	cached, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, cached, 1, "within the TTL the cached inventory should be reused")

	// Advancing the clock well past the (small) cache TTL forces a recompute,
	// and the new chassis appears.
	nowMs += (10 * time.Second).Milliseconds()
	fresh, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, fresh, 2, "after the TTL expires the inventory is recomputed")
}

// TestBuilderListEmpty verifies the empty-cache edge: no chassis yields an
// empty (non-nil) list and no error, even with no SB_Global row present.
func TestBuilderListEmpty(t *testing.T) {
	c := testutil.SetupSBTestClient(t)

	b := &inventory.Builder{SB: c, StaleThreshold: 60 * time.Second, Now: fixedNow}
	list, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestBuilderDetail covers the detail view: the embedded summary, the config
// copies, the sorted bound-port list, and the not-found error path.
func TestBuilderDetail(t *testing.T) {
	c := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, c, 7)
	chUUID := insertChassisWithConfig(t, c, "node-1", "node-1.example", "10.1.0.1", map[string]string{
		"ovn-bridge-mappings": "physnet1:br-ex",
		"datapath-type":       "netdev",
		"iface-types":         "dpdk,geneve",
		"ovn-cms-options":     "enable-chassis-as-gw,availability-zones=az0",
	})
	testutil.InsertChassisPrivate(t, c, "node-1", &chUUID, 7, int(t0-2000))

	// Insert ports out of name order to prove BoundPorts sorts by logical port.
	testutil.InsertPortBindingWithUp(t, c, "vif-z", "", &chUUID, boolPtr(true))
	testutil.InsertPortBindingWithUp(t, c, "vif-a", "localnet", &chUUID, boolPtr(false))
	testutil.InsertPortBinding(t, c, "unbound", "", nil)

	b := &inventory.Builder{SB: c, StaleThreshold: 60 * time.Second, Now: fixedNow}

	t.Run("found", func(t *testing.T) {
		detail, err := b.Detail(context.Background(), "node-1")
		require.NoError(t, err)

		// Embedded summary.
		assert.Equal(t, "node-1", detail.Name)
		assert.Equal(t, "physnet1:br-ex", detail.BridgeMappings)
		assert.True(t, detail.Liveness.InSync)
		assert.True(t, detail.Liveness.Alive)

		// Detail-only config copies, read from other_config.
		assert.Equal(t, "netdev", detail.OtherConfig["datapath-type"])
		assert.Equal(t, "dpdk,geneve", detail.OtherConfig["iface-types"])
		assert.Equal(t, "enable-chassis-as-gw,availability-zones=az0", detail.OtherConfig["ovn-cms-options"])
		assert.Equal(t, "physnet1:br-ex", detail.OtherConfig["ovn-bridge-mappings"])

		// Bound ports: sorted by logical port, unbound excluded.
		require.Len(t, detail.BoundPorts, 2)
		assert.Equal(t, "vif-a", detail.BoundPorts[0].LogicalPort)
		assert.Equal(t, "localnet", detail.BoundPorts[0].Type)
		require.NotNil(t, detail.BoundPorts[0].Up)
		assert.False(t, *detail.BoundPorts[0].Up)
		assert.Equal(t, "vif-z", detail.BoundPorts[1].LogicalPort)
		assert.Positive(t, detail.BoundPorts[1].TunnelKey)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := b.Detail(context.Background(), "does-not-exist")
		require.ErrorIs(t, err, inventory.ErrNotFound)
	})
}

// TestBuilderDetailMissingPrivate asserts that a chassis with no Chassis_Private
// row is reported not-in-sync and not-alive in the detail view as well.
func TestBuilderDetailMissingPrivate(t *testing.T) {
	c := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, c, 4)
	testutil.InsertChassis(t, c, "lonely", "lonely-host", "10.2.0.1")

	b := &inventory.Builder{SB: c, StaleThreshold: 60 * time.Second, Now: fixedNow}
	detail, err := b.Detail(context.Background(), "lonely")
	require.NoError(t, err)

	assert.False(t, detail.Liveness.InSync)
	assert.False(t, detail.Liveness.Alive)
	assert.Equal(t, 0, detail.Liveness.NbCfg)
	assert.Equal(t, 4, detail.Liveness.SBNbCfg)
	assert.Equal(t, int64(0), detail.Liveness.NbCfgTimestamp)
	assert.Empty(t, detail.BoundPorts)
}
