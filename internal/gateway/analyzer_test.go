package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
)

// gatewayByCR returns the analyzed gateway for the given cr-lrp port name.
func gatewayByCR(t *testing.T, rep *Report, crPort string) Gateway {
	t.Helper()
	for _, g := range rep.Gateways {
		if g.CRPort == crPort {
			return g
		}
	}
	t.Fatalf("gateway %q not found in report", crPort)
	return Gateway{}
}

// seedGateway wires an external network: LRP + router (+ optional NAT FIP) and a
// chassisredirect port bound to active, electing among the given HA members.
func seedGateway(t *testing.T, nbc, sbc client.Client, lrpName, network, fip string, active *string, members []testutil.HAChassisEntry) {
	t.Helper()
	grp := testutil.InsertHAChassisGroup(t, sbc, lrpName+"-grp", members)
	testutil.InsertChassisRedirectBinding(t, sbc, "cr-"+lrpName, active, grp.GroupUUID)

	var fips []string
	if fip != "" {
		fips = []string{fip}
	}
	testutil.InsertGatewayRouter(t, nbc, "router-"+lrpName, lrpName, []string{network}, fips)
}

// liveGen is the nb_cfg generation SB_Global advertises in the alive fixtures.
const liveGen = 5

// seedAlive marks a chassis in sync with SB_Global.nb_cfg (liveGen) via its
// Chassis_Private row, so gateway election treats it as alive. On OVN >= 20.06
// nb_cfg is written to Chassis_Private, not Chassis, so this is the correct
// liveness signal.
func seedAlive(t *testing.T, sbc client.Client, name, chassisUUID string) {
	t.Helper()
	testutil.InsertChassisPrivate(t, sbc, name, &chassisUUID, liveGen, 1)
}

// seedStale marks a chassis lagging the current nb_cfg generation with an old
// timestamp, so it is treated as stale (not alive) for election.
func seedStale(t *testing.T, sbc client.Client, name, chassisUUID string) {
	t.Helper()
	testutil.InsertChassisPrivate(t, sbc, name, &chassisUUID, 0, 1)
}

func TestAnalyze_Healthy(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
	// Modern OVN: both chassis are in sync via Chassis_Private (Chassis.nb_cfg is
	// left 0). Both must be alive — the regression against the false fleet-wide
	// no-viable-candidate that reading the deprecated Chassis.nb_cfg produced.
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1)
	seedAlive(t, sbc, "netnode-2", ch2)
	seedGateway(t, nbc, sbc, "lrp-ext", "10.10.141.1/24", "10.10.141.24", &ch1, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
		{ChassisUUID: ch2, Priority: 20},
	})

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, rep.Total)
	assert.Equal(t, 1, rep.Healthy)
	assert.Equal(t, 0, rep.Error)

	gw := gatewayByCR(t, rep, "cr-lrp-ext")
	assert.Equal(t, StatusOK, gw.Status)
	assert.Equal(t, SeverityHealthy, gw.Overall)
	assert.Equal(t, "netnode-1", gw.DesiredChassis)
	assert.Equal(t, "netnode-1", gw.ActualChassis)
	assert.Equal(t, "router-lrp-ext", gw.RouterName)
	assert.Contains(t, gw.ServedIPs, "10.10.141.1")
	assert.Contains(t, gw.ServedIPs, "10.10.141.24")

	// The active member is flagged; the standby is desired=false/active=false.
	var activeMember, desiredMember Member
	for _, m := range gw.Members {
		if m.Active {
			activeMember = m
		}
		if m.Desired {
			desiredMember = m
		}
	}
	assert.Equal(t, "netnode-1", activeMember.Name)
	assert.Equal(t, "netnode-1", desiredMember.Name)

	// Both in-sync members must be alive and not stale.
	for _, m := range gw.Members {
		assert.True(t, m.Alive, "member %s must be alive", m.Name)
		assert.False(t, m.Stale, "member %s must not be stale", m.Name)
	}
}

func TestAnalyze_FailoverStuck(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1)
	seedAlive(t, sbc, "netnode-2", ch2)
	// Bound to the LOWER-priority chassis while the higher one is alive.
	seedGateway(t, nbc, sbc, "lrp-ext", "10.10.141.1/24", "10.10.141.24", &ch2, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
		{ChassisUUID: ch2, Priority: 20},
	})

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	gw := gatewayByCR(t, rep, "cr-lrp-ext")
	assert.Equal(t, StatusFailoverStuck, gw.Status)
	assert.Equal(t, SeverityError, gw.Overall)
	assert.Equal(t, "netnode-1", gw.DesiredChassis)
	assert.Equal(t, "netnode-2", gw.ActualChassis)
	assert.Equal(t, 1, rep.Error)

	var election Check
	for _, c := range gw.Checks {
		if c.Name == "election" {
			election = c
		}
	}
	assert.Equal(t, SeverityError, election.Status)
	assert.Contains(t, election.Message, "failover stuck")
}

func TestAnalyze_NoOwner(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1)
	// Active chassis is nil -> blackhole.
	seedGateway(t, nbc, sbc, "lrp-ext", "10.10.141.1/24", "", nil, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
	})

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	gw := gatewayByCR(t, rep, "cr-lrp-ext")
	assert.Equal(t, StatusNoOwner, gw.Status)
	assert.Equal(t, SeverityError, gw.Overall)
	assert.Empty(t, gw.ActualChassis)
}

func TestAnalyze_NoCandidate(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)
	// SB_Global generation far ahead of the chassis, and their Chassis_Private
	// timestamps are old -> all members stale -> not alive.
	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
	testutil.InsertSBGlobal(t, sbc, 50)
	seedStale(t, sbc, "netnode-1", ch1)
	seedStale(t, sbc, "netnode-2", ch2)
	seedGateway(t, nbc, sbc, "lrp-ext", "10.10.141.1/24", "", &ch1, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
		{ChassisUUID: ch2, Priority: 20},
	})

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	gw := gatewayByCR(t, rep, "cr-lrp-ext")
	assert.Equal(t, StatusNoCandidate, gw.Status)
	assert.Equal(t, SeverityError, gw.Overall)
	assert.Empty(t, gw.DesiredChassis)
	for _, m := range gw.Members {
		assert.True(t, m.Stale, "member %s should be stale", m.Name)
		assert.False(t, m.Alive)
	}
}

func TestAnalyze_Degraded(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	testutil.InsertSBGlobal(t, sbc, 50)
	seedStale(t, sbc, "netnode-1", ch1) // ch1 lagging -> stale
	// No HA group and no gateway-chassis list -> pinned to a single (stale) chassis.
	testutil.InsertChassisRedirectBinding(t, sbc, "cr-lrp-solo", &ch1, "")
	testutil.InsertGatewayRouter(t, nbc, "router-solo", "lrp-solo", []string{"10.10.150.1/24"}, nil)

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	gw := gatewayByCR(t, rep, "cr-lrp-solo")
	assert.Equal(t, StatusDegraded, gw.Status)
	assert.Equal(t, SeverityWarning, gw.Overall)
	assert.Equal(t, 1, rep.Warning)
	assert.Empty(t, gw.Members)
}

func TestAnalyze_NotGWCapable(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1)
	setCMSOptions(t, sbc, ch1, "enable-chassis-as-gw=false") // present but not a gateway
	seedGateway(t, nbc, sbc, "lrp-ext", "10.10.141.1/24", "", &ch1, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
	})

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	gw := gatewayByCR(t, rep, "cr-lrp-ext")
	var cap Check
	for _, c := range gw.Checks {
		if c.Name == "gateway_capability" {
			cap = c
		}
	}
	assert.Equal(t, SeverityWarning, cap.Status)
	assert.Equal(t, SeverityWarning, gw.Overall)
}

func TestAnalyze_Conflict(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1)
	seedAlive(t, sbc, "netnode-2", ch2)

	// Two routers, each healthy on its own chassis, but both serve the same FIP.
	seedGateway(t, nbc, sbc, "lrp-a", "10.10.141.1/24", "10.10.141.24", &ch1, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
	})
	seedGateway(t, nbc, sbc, "lrp-b", "10.10.142.1/24", "10.10.141.24", &ch2, []testutil.HAChassisEntry{
		{ChassisUUID: ch2, Priority: 30},
	})

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	require.Len(t, rep.Conflicts, 1)
	c := rep.Conflicts[0]
	assert.Equal(t, "10.10.141.24", c.ExternalIP)
	assert.ElementsMatch(t, []string{"netnode-1", "netnode-2"}, c.Chassis)
	assert.ElementsMatch(t, []string{"cr-lrp-a", "cr-lrp-b"}, c.CRPorts)
}

func TestAnalyze_GWList(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1)
	seedAlive(t, sbc, "netnode-2", ch2)
	// Legacy gateway_chassis list instead of an HA group; bound to lower priority.
	insertGatewayChassisBinding(t, sbc, "cr-lrp-legacy", &ch2, []gcEntry{
		{chassis: ch1, prio: 30},
		{chassis: ch2, prio: 20},
	})
	testutil.InsertGatewayRouter(t, nbc, "router-legacy", "lrp-legacy", []string{"10.10.160.1/24"}, nil)

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	gw := gatewayByCR(t, rep, "cr-lrp-legacy")
	assert.Equal(t, StatusFailoverStuck, gw.Status)
	assert.Equal(t, "netnode-1", gw.DesiredChassis)
	assert.Equal(t, "netnode-2", gw.ActualChassis)
	assert.Len(t, gw.Members, 2)
}

func TestAnalyze_MissingChassisPrivateNotAlive(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1) // ch1 has a controller heartbeat
	// ch2 has NO Chassis_Private row -> no heartbeat -> not alive.
	seedGateway(t, nbc, sbc, "lrp-ext", "10.10.141.1/24", "", &ch1, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
		{ChassisUUID: ch2, Priority: 20},
	})

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)

	gw := gatewayByCR(t, rep, "cr-lrp-ext")
	byName := map[string]Member{}
	for _, m := range gw.Members {
		byName[m.Name] = m
	}
	assert.True(t, byName["netnode-1"].Alive, "chassis with a heartbeat must be alive")
	assert.False(t, byName["netnode-2"].Alive, "chassis without Chassis_Private must not be alive")
	assert.True(t, byName["netnode-2"].Stale)
}

func TestAnalyze_NoGateways(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)
	// A regular VIF port binding must be ignored.
	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	testutil.InsertPortBinding(t, sbc, "vif-1", "", &ch1)

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, rep.Total)
	assert.Empty(t, rep.Gateways)
	assert.Empty(t, rep.Conflicts)

	// Gateways must marshal as a non-nil JSON array (it has no omitempty), so an
	// empty report is "gateways":[] and never null.
	raw, err := json.Marshal(rep)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"gateways":[]`)
}

func TestAnalyze_Memoized(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	testutil.InsertSBGlobal(t, sbc, liveGen)
	seedAlive(t, sbc, "netnode-1", ch1)
	seedGateway(t, nbc, sbc, "lrp-ext", "10.10.141.1/24", "", &ch1, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
	})

	base := time.Unix(1_700_000_000, 0)
	now := base
	a := &Analyzer{NB: nbc, SB: sbc, Now: func() time.Time { return now }}

	first, err := a.Analyze(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, first.Total)

	// Add a second gateway; within the TTL window the cached report is returned
	// unchanged (same pointer, same snapshot) even though the DB now differs.
	seedGateway(t, nbc, sbc, "lrp-ext2", "10.10.142.1/24", "", &ch1, []testutil.HAChassisEntry{
		{ChassisUUID: ch1, Priority: 30},
	})
	now = base.Add(analyzeCacheTTL - time.Millisecond)
	second, err := a.Analyze(context.Background())
	require.NoError(t, err)
	assert.Same(t, first, second, "within the TTL the cached report pointer is reused")
	assert.Equal(t, 1, second.Total, "cached snapshot must not reflect the new gateway")

	// Advance past the TTL: the report is recomputed and now sees both gateways.
	now = base.Add(analyzeCacheTTL + time.Millisecond)
	third, err := a.Analyze(context.Background())
	require.NoError(t, err)
	assert.NotSame(t, first, third, "past the TTL a fresh report is computed")
	assert.Equal(t, 2, third.Total)
}

// --- local helpers for the legacy gateway_chassis path ---

type gcEntry struct {
	chassis string
	prio    int
}

// insertGatewayChassisBinding creates Gateway_Chassis rows and a chassisredirect
// Port_Binding referencing them via its gateway_chassis column.
func insertGatewayChassisBinding(t *testing.T, c client.Client, logicalPort string, active *string, entries []gcEntry) {
	t.Helper()
	var allOps []ovsdb.Operation
	named := make([]string, len(entries))
	for i, e := range entries {
		named[i] = "gc_" + logicalPort + "_" + string(rune('a'+i))
		chassis := e.chassis
		gc := &sb.GatewayChassis{
			UUID:        named[i],
			Name:        named[i],
			Chassis:     &chassis,
			Priority:    e.prio,
			Options:     map[string]string{},
			ExternalIDs: map[string]string{},
		}
		ops, err := c.Create(gc)
		require.NoError(t, err)
		allOps = append(allOps, ops...)
	}
	dpUUID := testutil.InsertDatapathBinding(t, c)
	pb := &sb.PortBinding{
		LogicalPort:    logicalPort,
		Type:           "chassisredirect",
		Datapath:       dpUUID,
		Chassis:        active,
		GatewayChassis: named,
		TunnelKey:      9000 + len(entries),
		ExternalIDs:    map[string]string{},
		Options:        map[string]string{},
	}
	ops, err := c.Create(pb)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)
}

// setCMSOptions sets ovn-cms-options in a chassis's other_config.
func setCMSOptions(t *testing.T, c client.Client, chassisUUID, opts string) {
	t.Helper()
	ch := &sb.Chassis{UUID: chassisUUID}
	require.NoError(t, c.Get(context.Background(), ch))
	ch.OtherConfig = map[string]string{"ovn-cms-options": opts}
	ops, err := c.Where(ch).Update(ch, &ch.OtherConfig)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
}
