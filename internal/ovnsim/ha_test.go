package ovnsim

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func threeChassis() []string { return []string{"chassis-1", "chassis-2", "chassis-3"} }

// haPriorities reads the current priority of every owned HA_Chassis keyed by
// chassis name (the seed uses one member per distinct chassis).
func haPriorities(t *testing.T, c client.Client) map[string]int {
	t.Helper()
	var rows []nb.HAChassis
	require.NoError(t, c.List(context.Background(), &rows))
	out := map[string]int{}
	for _, h := range rows {
		if owned(h.ExternalIDs) {
			out[h.ChassisName] = h.Priority
		}
	}
	return out
}

func TestSeedCreatesHAGroupOnEvenRouter(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 2, PortsPerSwitch: 1, Chassis: threeChassis()})
	require.NoError(t, err)
	eventuallyCount[nb.HAChassisGroup](t, c, 1)
	eventuallyCount[nb.HAChassis](t, c, 3)

	var groups []nb.HAChassisGroup
	require.NoError(t, c.List(context.Background(), &groups))
	require.Len(t, groups, 1)
	assert.Equal(t, haGroupName(2), groups[0].Name)
	assert.Len(t, groups[0].HaChassis, 3)

	// The even router's port references the HA group; the odd router's port uses
	// a per-port Gateway_Chassis instead.
	var lrps []nb.LogicalRouterPort
	require.NoError(t, c.List(context.Background(), &lrps))
	byName := map[string]nb.LogicalRouterPort{}
	for _, p := range lrps {
		byName[p.Name] = p
	}
	even := byName[lrpName(2, 2)]
	require.NotNil(t, even.HaChassisGroup)
	assert.Equal(t, groups[0].UUID, *even.HaChassisGroup)
	assert.Empty(t, even.GatewayChassis)

	odd := byName[lrpName(1, 1)]
	assert.Nil(t, odd.HaChassisGroup)
	assert.NotEmpty(t, odd.GatewayChassis)
}

func TestHAFailoverSwapsHighestAndLowest(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 2, PortsPerSwitch: 1, Chassis: threeChassis()})
	require.NoError(t, err)
	eventuallyCount[nb.HAChassis](t, c, 3)

	// Seeded priorities: chassis-1=100, chassis-2=90, chassis-3=80.
	before := haPriorities(t, c)
	require.Equal(t, 100, before["chassis-1"])
	require.Equal(t, 80, before["chassis-3"])

	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 2, Routers: 2, PortsPerSwitch: 1, Chassis: threeChassis()}})
	desc, err := sim.haFailover(context.Background())
	require.NoError(t, err)
	assert.Contains(t, desc, "HA failover")

	// The highest- and lowest-priority members swapped; the middle is untouched.
	require.Eventually(t, func() bool {
		p := haPriorities(t, c)
		return p["chassis-1"] == 80 && p["chassis-3"] == 100 && p["chassis-2"] == 90
	}, 3*time.Second, 20*time.Millisecond, "priorities did not swap")
}

func TestHAMemberChurnRemoves(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 2, PortsPerSwitch: 1, Chassis: threeChassis()})
	require.NoError(t, err)
	eventuallyCount[nb.HAChassis](t, c, 3)

	// All three chassis are already members, so there is nothing to add and churn
	// must remove a member (the orphaned HA_Chassis row is garbage-collected).
	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 2, Routers: 2, PortsPerSwitch: 1, Chassis: threeChassis()}})
	desc, err := sim.haMemberChurn(context.Background())
	require.NoError(t, err)
	assert.Contains(t, desc, "remove chassis")
	eventuallyCount[nb.HAChassis](t, c, 2)
}

func TestHAMemberChurnAdds(t *testing.T) {
	c := setupNB(t)
	// Seed with a single chassis, so the HA group has one member and churn cannot
	// remove (must keep >=1) — forcing an add of the new chassis.
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 2, PortsPerSwitch: 1, Chassis: []string{"chassis-1"}})
	require.NoError(t, err)
	eventuallyCount[nb.HAChassis](t, c, 1)

	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 2, Routers: 2, PortsPerSwitch: 1, Chassis: []string{"chassis-1", "chassis-2"}}})
	desc, err := sim.haMemberChurn(context.Background())
	require.NoError(t, err)
	assert.Contains(t, desc, "add chassis chassis-2")
	eventuallyCount[nb.HAChassis](t, c, 2)
}
