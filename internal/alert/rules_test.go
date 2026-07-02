package alert

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaleChassis(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, sbClient, 10)
	chUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	// Chassis_Private lags SB_Global (nb_cfg 0 vs 10) with an old timestamp.
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chUUID, 0, 1)

	rule := StaleChassis(sbClient, 30*time.Second)
	alerts := rule.Check(context.Background())
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
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)
}

func TestStaleChassis_MissingPrivate(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertSBGlobal(t, sbClient, 5)
	testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1") // no Chassis_Private row

	rule := StaleChassis(sbClient, 30*time.Second)
	alerts := rule.Check(context.Background())
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
	alerts := rule.Check(context.Background())
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
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)
}

func TestPortDown_SkipsVirtual(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	down := false
	testutil.InsertPortBindingWithUp(t, sbClient, "virtual-port", "virtual", &chassisUUID, &down)

	rule := PortDown(sbClient)
	alerts := rule.Check(context.Background())
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
	alerts := rule.Check(context.Background())
	require.Len(t, alerts, 1)
	assert.Equal(t, "unbound_port", alerts[0].Rule)
	assert.Contains(t, alerts[0].Message, "unbound-vif")
}

func TestUnboundPort_AllBound(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertPortBinding(t, sbClient, "bound-vif", "", &chassisUUID)

	rule := UnboundPort(sbClient)
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)
}

func TestUnboundPort_SkipsNonVIF(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	// Non-VIF port (type="l3gateway") with no chassis — should not alert
	testutil.InsertPortBinding(t, sbClient, "gw-port", "l3gateway", nil)

	rule := UnboundPort(sbClient)
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)
}

func TestBFDDown(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertBFD(t, sbClient, "ch-1", "10.0.0.2", "port-1", sb.BFDStatusDown)
	testutil.InsertBFD(t, sbClient, "ch-1", "10.0.0.3", "port-2", sb.BFDStatusUp)

	rule := BFDDown(sbClient)
	alerts := rule.Check(context.Background())
	require.Len(t, alerts, 1)
	assert.Equal(t, SeverityCritical, alerts[0].Severity)
	assert.Contains(t, alerts[0].Message, "10.0.0.2")
}

func TestBFDDown_NoneDown(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertBFD(t, sbClient, "ch-1", "10.0.0.2", "port-1", sb.BFDStatusUp)

	rule := BFDDown(sbClient)
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)
}

func TestFlowCountAnomaly(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	rule := FlowCountAnomaly(sbClient, 20.0)

	// First run — baseline
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)

	// Second run — no change
	alerts = rule.Check(context.Background())
	assert.Empty(t, alerts)
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
	alerts := rule.Check(context.Background())
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
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)

	// Second run — no change
	alerts = rule.Check(context.Background())
	assert.Empty(t, alerts)
}

func TestHAFailover_EmptyGroups(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)

	rule := HAFailover(sbClient)

	// First run with no HA groups — should not panic
	alerts := rule.Check(context.Background())
	assert.Empty(t, alerts)

	// Second run — still no groups
	alerts = rule.Check(context.Background())
	assert.Empty(t, alerts)
}
