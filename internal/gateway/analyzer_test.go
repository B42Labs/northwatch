package gateway

import (
	"context"
	"testing"

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

func TestAnalyze_Healthy(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)
	testutil.InsertNBGlobal(t, nbc, 0, 0, 0)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
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
}

func TestAnalyze_FailoverStuck(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)
	testutil.InsertNBGlobal(t, nbc, 0, 0, 0)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
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
	testutil.InsertNBGlobal(t, nbc, 0, 0, 0)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
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
	// NB generation far ahead of the chassis -> all members stale -> not alive.
	testutil.InsertNBGlobal(t, nbc, 50, 0, 0)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
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
	testutil.InsertNBGlobal(t, nbc, 50, 0, 0) // ch1 stale

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
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
	testutil.InsertNBGlobal(t, nbc, 0, 0, 0)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
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
	testutil.InsertNBGlobal(t, nbc, 0, 0, 0)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")

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
	testutil.InsertNBGlobal(t, nbc, 0, 0, 0)

	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
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

func TestAnalyze_NoGateways(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)
	testutil.InsertNBGlobal(t, nbc, 0, 0, 0)
	// A regular VIF port binding must be ignored.
	ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
	testutil.InsertPortBinding(t, sbc, "vif-1", "", &ch1)

	rep, err := (&Analyzer{NB: nbc, SB: sbc}).Analyze(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, rep.Total)
	assert.Empty(t, rep.Gateways)
	assert.Empty(t, rep.Conflicts)
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
