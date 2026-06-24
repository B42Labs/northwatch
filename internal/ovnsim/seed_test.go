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

func countTable[T any](t *testing.T, c client.Client) int {
	t.Helper()
	var rows []T
	require.NoError(t, c.List(context.Background(), &rows))
	return len(rows)
}

// eventuallyCount waits for a table to reach the expected row count, accounting
// for the libovsdb cache catching up after a transaction.
func eventuallyCount[T any](t *testing.T, c client.Client, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return countTable[T](t, c) == want
	}, 3*time.Second, 20*time.Millisecond, "table never reached %d rows", want)
}

func TestSeedCreatesBaseline(t *testing.T) {
	c := setupNB(t)
	opts := Options{Switches: 4, Routers: 2, PortsPerSwitch: 3, Chassis: []string{"chassis-1", "chassis-2", "chassis-3"}}

	res, err := Seed(context.Background(), c, opts)
	require.NoError(t, err)

	// Result counts reflect what was created.
	assert.Equal(t, 4, res.Created["Logical_Switch"])
	assert.Equal(t, 2, res.Created["Logical_Router"])
	assert.Equal(t, 16, res.Created["Logical_Switch_Port"]) // (3 vif + 1 uplink) * 4
	assert.Equal(t, 4, res.Created["Logical_Router_Port"])
	assert.Equal(t, 4, res.Created["NAT"])
	assert.Equal(t, 4, res.Created["DHCP_Options"])
	// Each router has exactly one distributed gateway port: odd router (1) uses a
	// single Gateway_Chassis, even router (2) uses one HA_Chassis_Group with one
	// member per chassis. The other router ports are plain patch ports.
	assert.Equal(t, 1, res.Created["Gateway_Chassis"])
	assert.Equal(t, 1, res.Created["HA_Chassis_Group"])
	assert.Equal(t, 3, res.Created["HA_Chassis"])
	assert.Equal(t, 6, res.Created["ACL"]) // 1 per switch + 2 in the port group
	assert.Equal(t, 1, res.Created["Port_Group"])
	assert.Equal(t, 2, res.Created["Load_Balancer"])
	assert.Equal(t, 1, res.Created["Meter"])
	assert.Equal(t, 1, res.Created["DNS"])

	// The rows actually landed in the database. (Counts are checked via
	// eventuallyCount because the libovsdb cache reflects each transaction
	// slightly after it commits.)
	eventuallyCount[nb.LogicalSwitch](t, c, 4)
	eventuallyCount[nb.LogicalRouter](t, c, 2)
	eventuallyCount[nb.LogicalSwitchPort](t, c, 16)
	eventuallyCount[nb.LoadBalancer](t, c, 2)

	// Non-root children survived garbage collection because a root references them.
	eventuallyCount[nb.ACL](t, c, 6) // 1 per switch + 2 in the port group
	eventuallyCount[nb.NAT](t, c, 4)
	eventuallyCount[nb.QoS](t, c, 4)
	eventuallyCount[nb.MeterBand](t, c, 1)
}

func TestSeedIsIdempotent(t *testing.T) {
	c := setupNB(t)
	opts := Options{Switches: 3, Routers: 2, PortsPerSwitch: 2}

	first, err := Seed(context.Background(), c, opts)
	require.NoError(t, err)
	require.Positive(t, first.Total())

	// Wait for the cache to reflect the first seed before re-seeding, otherwise
	// the idempotency check would not see the existing names.
	eventuallyCount[nb.LogicalSwitch](t, c, 3)
	eventuallyCount[nb.LoadBalancer](t, c, 2)

	second, err := Seed(context.Background(), c, opts)
	require.NoError(t, err)
	assert.Zero(t, second.Total(), "re-seeding must create nothing")

	// Totals unchanged.
	assert.Equal(t, 3, countTable[nb.LogicalSwitch](t, c))
	assert.Equal(t, 2, countTable[nb.LogicalRouter](t, c))
}

func TestSeedConnectsSwitchesToRouters(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 1, PortsPerSwitch: 1})
	require.NoError(t, err)

	eventuallyCount[nb.LogicalSwitch](t, c, 2)

	// Every switch has a router-type uplink port pointing at an LRP that exists.
	var lrps []nb.LogicalRouterPort
	require.NoError(t, c.List(context.Background(), &lrps))
	lrpNames := map[string]bool{}
	for _, p := range lrps {
		lrpNames[p.Name] = true
	}

	var ports []nb.LogicalSwitchPort
	require.NoError(t, c.List(context.Background(), &ports))
	uplinks := 0
	for _, p := range ports {
		if p.Type == "router" {
			uplinks++
			assert.True(t, lrpNames[p.Options["router-port"]], "uplink %s references missing LRP %s", p.Name, p.Options["router-port"])
		}
	}
	assert.Equal(t, 2, uplinks)
}
