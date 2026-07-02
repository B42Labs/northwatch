package alert

import (
	"context"
	"testing"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertLogicalFlow adds a minimal Logical_Flow (a root table with only scalar
// required columns) and returns its UUID.
func insertLogicalFlow(t *testing.T, c client.Client, match string) {
	t.Helper()
	lf := &sb.LogicalFlow{Pipeline: "ingress", TableID: 0, Priority: 100, Match: match, Actions: "next;"}
	ops, err := c.Create(lf)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.LogicalFlow{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
}

// deleteAllLogicalFlows removes every Logical_Flow row.
func deleteAllLogicalFlows(t *testing.T, c client.Client) {
	t.Helper()
	ops, err := c.WhereCache(func(*sb.LogicalFlow) bool { return true }).Delete()
	require.NoError(t, err)
	if len(ops) == 0 {
		return
	}
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var flows []sb.LogicalFlow
		return c.List(context.Background(), &flows) == nil && len(flows) == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestStaleChassis(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, sbClient, 10)
	chUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	// Chassis_Private lags SB_Global (nb_cfg 0 vs 10) with an old timestamp.
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chUUID, 0, 1)

	rule := StaleChassis(sbClient, 30*time.Second)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	require.Len(t, alerts, 1)
	assert.Equal(t, "stale_chassis_config", alerts[0].Rule)
	assert.Contains(t, alerts[0].Message, "ch-1")
	assert.Equal(t, SeverityWarning, alerts[0].Severity)
}

func TestStaleChassis_InSync(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, sbClient, 5)
	chUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chUUID, 5, 1) // acknowledged current gen

	rule := StaleChassis(sbClient, 30*time.Second)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestStaleChassis_MissingPrivate(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, sbClient, 5)
	testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1") // no Chassis_Private row

	rule := StaleChassis(sbClient, 30*time.Second)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	require.Len(t, alerts, 1)
	assert.Contains(t, alerts[0].Message, "no ovn-controller heartbeat")
}

func TestPortDown(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	down := false
	testutil.InsertPortBindingWithUp(t, sbClient, "port-down", "", &chassisUUID, &down)
	up := true
	testutil.InsertPortBindingWithUp(t, sbClient, "port-up", "", &chassisUUID, &up)

	rule := PortDown(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	require.Len(t, alerts, 1)
	assert.Equal(t, "port_down", alerts[0].Rule)
	assert.Contains(t, alerts[0].Message, "port-down")
}

func TestPortDown_NoneDown(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	up := true
	testutil.InsertPortBindingWithUp(t, sbClient, "port-up", "", &chassisUUID, &up)

	rule := PortDown(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestPortDown_SkipsVirtual(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	down := false
	testutil.InsertPortBindingWithUp(t, sbClient, "virtual-port", "virtual", &chassisUUID, &down)

	rule := PortDown(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestUnboundPort(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	// VIF port (type="") with no chassis
	testutil.InsertPortBinding(t, sbClient, "unbound-vif", "", nil)

	// VIF port with chassis
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertPortBinding(t, sbClient, "bound-vif", "", &chassisUUID)

	rule := UnboundPort(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	require.Len(t, alerts, 1)
	assert.Equal(t, "unbound_port", alerts[0].Rule)
	assert.Contains(t, alerts[0].Message, "unbound-vif")
}

func TestUnboundPort_AllBound(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertPortBinding(t, sbClient, "bound-vif", "", &chassisUUID)

	rule := UnboundPort(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestUnboundPort_SkipsNonVIF(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	// Non-VIF port (type="l3gateway") with no chassis — should not alert
	testutil.InsertPortBinding(t, sbClient, "gw-port", "l3gateway", nil)

	rule := UnboundPort(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestBFDDown(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertBFD(t, sbClient, "ch-1", "10.0.0.2", "port-1", sb.BFDStatusDown)
	testutil.InsertBFD(t, sbClient, "ch-1", "10.0.0.3", "port-2", sb.BFDStatusUp)

	rule := BFDDown(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	require.Len(t, alerts, 1)
	assert.Equal(t, SeverityCritical, alerts[0].Severity)
	assert.Contains(t, alerts[0].Message, "10.0.0.2")
}

func TestBFDDown_NoneDown(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertBFD(t, sbClient, "ch-1", "10.0.0.2", "port-1", sb.BFDStatusUp)

	rule := BFDDown(sbClient)
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestFlowCountAnomaly(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	rule := FlowCountAnomaly(sbClient, 20.0)

	// First run — baseline
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)

	// Second run — no change
	alerts, checkErr = rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

// TestFlowCountAnomaly_ZeroGuard verifies a purge of the flow table to zero — and
// the subsequent repopulation — does not fire, since a zero count is
// indistinguishable from a monitoring gap and must not be treated as a 100% drop.
func TestFlowCountAnomaly_ZeroGuard(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)
	rule := FlowCountAnomaly(sbClient, 20.0)

	// Establish a baseline of two flows.
	insertLogicalFlow(t, sbClient, "ip4.dst==10.0.0.1")
	insertLogicalFlow(t, sbClient, "ip4.dst==10.0.0.2")
	alerts, err := rule.Check(context.Background())
	require.NoError(t, err)
	require.Empty(t, alerts) // first run: baseline

	// Purge to zero — must NOT fire and must NOT rebaseline off zero.
	deleteAllLogicalFlows(t, sbClient)
	alerts, err = rule.Check(context.Background())
	require.NoError(t, err)
	assert.Empty(t, alerts, "a drop to zero must not fire")

	// Repopulate to the baseline — must NOT fire (baseline was kept at two).
	insertLogicalFlow(t, sbClient, "ip4.dst==10.0.0.1")
	insertLogicalFlow(t, sbClient, "ip4.dst==10.0.0.2")
	alerts, err = rule.Check(context.Background())
	require.NoError(t, err)
	assert.Empty(t, alerts, "repopulation to the baseline must not fire")
}

func TestHAFailover_FirstRunNoAlert(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	ch1UUID := testutil.InsertChassis(t, sbClient, "gw-1", "host-1", "10.0.0.1")
	ch2UUID := testutil.InsertChassis(t, sbClient, "gw-2", "host-2", "10.0.0.2")

	testutil.InsertHAChassisGroup(t, sbClient, "ha-group-1", []testutil.HAChassisEntry{
		{ChassisUUID: ch1UUID, Priority: 100},
		{ChassisUUID: ch2UUID, Priority: 50},
	})

	rule := HAFailover(sbClient)

	// First run — initialization, no alerts
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestHAFailover_NoChangeNoAlert(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	ch1UUID := testutil.InsertChassis(t, sbClient, "gw-1", "host-1", "10.0.0.1")
	ch2UUID := testutil.InsertChassis(t, sbClient, "gw-2", "host-2", "10.0.0.2")

	testutil.InsertHAChassisGroup(t, sbClient, "ha-group-1", []testutil.HAChassisEntry{
		{ChassisUUID: ch1UUID, Priority: 100},
		{ChassisUUID: ch2UUID, Priority: 50},
	})

	rule := HAFailover(sbClient)

	// First run — initialization
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)

	// Second run — no change
	alerts, checkErr = rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}

func TestHAFailover_EmptyGroups(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	rule := HAFailover(sbClient)

	// First run with no HA groups — should not panic
	alerts, checkErr := rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)

	// Second run — still no groups
	alerts, checkErr = rule.Check(context.Background())
	require.NoError(t, checkErr)
	assert.Empty(t, alerts)
}
