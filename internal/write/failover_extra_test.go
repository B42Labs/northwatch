package write

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatewayChassisSpec describes a Gateway_Chassis entry to create under an LRP.
type gatewayChassisSpec struct {
	chassisName string
	priority    int
}

// insertGatewayChassisGroup creates a Logical_Router that owns one
// Logical_Router_Port referencing the given Gateway_Chassis entries, all in a
// single transaction (the router port and gateway chassis are non-root and
// would otherwise be garbage-collected). Returns the LRP UUID and the real
// Gateway_Chassis UUIDs in spec order.
func insertGatewayChassisGroup(t *testing.T, c client.Client, lrpName string, specs []gatewayChassisSpec) (string, []string) {
	t.Helper()

	var allOps []ovsdb.Operation
	gcNamed := make([]string, len(specs))
	for i, s := range specs {
		gcNamed[i] = fmt.Sprintf("gc-%d", i)
		gc := &nb.GatewayChassis{
			UUID:        gcNamed[i],
			Name:        s.chassisName + "-gc",
			ChassisName: s.chassisName,
			Priority:    s.priority,
			ExternalIDs: map[string]string{},
			Options:     map[string]string{},
		}
		ops, err := c.Create(gc)
		require.NoError(t, err)
		allOps = append(allOps, ops...)
	}

	lrp := &nb.LogicalRouterPort{
		UUID:           "lrp-gw",
		Name:           lrpName,
		MAC:            "00:00:00:00:00:01",
		Networks:       []string{"10.0.0.1/24"},
		GatewayChassis: gcNamed,
		ExternalIDs:    map[string]string{},
		Options:        map[string]string{},
		Ipv6RaConfigs:  map[string]string{},
		Status:         map[string]string{},
	}
	ops, err := c.Create(lrp)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	lr := &nb.LogicalRouter{
		Name:        "lr-" + lrpName,
		Ports:       []string{"lrp-gw"},
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	ops, err = c.Create(lr)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	gcUUIDs := make([]string, len(specs))
	for i := range specs {
		gcUUIDs[i] = reply[i].UUID.GoUUID
	}
	lrpUUID := reply[len(specs)].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LogicalRouterPort{UUID: lrpUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return lrpUUID, gcUUIDs
}

// TestFailoverFallbackSingleCandidate covers resolveGroupUnified's fallback:
// when the group name does not match any group, the single group containing the
// target chassis is used.
func TestFailoverFallbackSingleCandidate(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	insertHAGroup(t, nbClient, "real-name", []haChassisSpec{
		{"chassis-x", 50, nil},
		{"chassis-a", 100, nil},
	})
	engine := setupEngineWithClients(t, nbClient, nil)

	plan, err := engine.Failover(context.Background(), "wrong-name", "chassis-x")
	require.NoError(t, err)
	assert.Equal(t, "pending", plan.Status)
	assert.Len(t, plan.Operations, 2, "the containing group is resolved by chassis membership")
}

// TestFailoverAmbiguousWithoutSB covers the ambiguous fallback: the target
// chassis appears in multiple groups and, without an SB client to
// disambiguate, an InputError is returned.
func TestFailoverAmbiguousWithoutSB(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	insertHAGroup(t, nbClient, "nb-group-1", []haChassisSpec{
		{"chassis-x", 50, nil},
		{"chassis-a", 100, nil},
	})
	insertHAGroup(t, nbClient, "nb-group-2", []haChassisSpec{
		{"chassis-x", 50, nil},
		{"chassis-b", 100, nil},
	})
	engine := setupEngineWithClients(t, nbClient, nil)

	_, err := engine.Failover(context.Background(), "does-not-exist", "chassis-x")
	require.Error(t, err)
	assert.True(t, IsInputError(err))
	assert.Contains(t, err.Error(), "appears in")
}

// TestFailoverSBDisambiguation covers disambiguateViaSBUnified: the target
// chassis is in two NB groups, and the SB HA_Chassis_Group with the requested
// name selects the matching one by its chassis-name set.
func TestFailoverSBDisambiguation(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	ctx := context.Background()

	// Two NB groups both containing chassis-x, distinguishable by their other
	// member (chassis-a vs chassis-b).
	insertHAGroup(t, nbClient, "nb-group-1", []haChassisSpec{
		{"chassis-x", 50, nil},
		{"chassis-a", 100, nil},
	})
	insertHAGroup(t, nbClient, "nb-group-2", []haChassisSpec{
		{"chassis-x", 50, nil},
		{"chassis-b", 100, nil},
	})

	// SB group "sb-target" resolves to {chassis-x, chassis-a}, matching
	// nb-group-1.
	cxUUID := testutil.InsertChassis(t, sbClient, "chassis-x", "host-x", "10.0.0.1")
	caUUID := testutil.InsertChassis(t, sbClient, "chassis-a", "host-a", "10.0.0.2")
	testutil.InsertHAChassisGroup(t, sbClient, "sb-target", []testutil.HAChassisEntry{
		{ChassisUUID: cxUUID, Priority: 50},
		{ChassisUUID: caUUID, Priority: 100},
	})

	engine := setupEngineWithClients(t, nbClient, sbClient)

	plan, err := engine.Failover(ctx, "sb-target", "chassis-x")
	require.NoError(t, err)
	assert.Equal(t, "pending", plan.Status)
	// nb-group-1: active is chassis-a (100), target chassis-x (50) -> swap.
	require.Len(t, plan.Operations, 2)
	for _, op := range plan.Operations {
		assert.Equal(t, "HA_Chassis", op.Table)
	}
}

// TestEvacuateGatewayChassis covers the Gateway_Chassis arm of collectAllGroups
// by draining a chassis that participates in an LRP's gateway-chassis set.
func TestEvacuateGatewayChassis(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	_, gcUUIDs := insertGatewayChassisGroup(t, nbClient, "lrp-gw-evac", []gatewayChassisSpec{
		{"gw-chassis-1", 100},
		{"gw-chassis-2", 50},
	})
	engine := setupEngineWithClients(t, nbClient, nil)

	plan, err := engine.Evacuate(context.Background(), "gw-chassis-1")
	require.NoError(t, err)
	require.Len(t, plan.Operations, 1)
	assert.Equal(t, "Gateway_Chassis", plan.Operations[0].Table)
	assert.Equal(t, gcUUIDs[0], plan.Operations[0].UUID)
	assert.Equal(t, 0, plan.Operations[0].Fields["priority"])
	extIDs, ok := plan.Operations[0].Fields["external_ids"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "100", extIDs[drainPriorityKey])
}
