package ovnsim

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindAllBindsEveryVIFRoundRobin(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 1, PortsPerSwitch: 3})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitchPort](t, c, 2*(3+1)) // 3 vif + 1 uplink per switch

	binder, calls := newRecordingBinder() // chassis-1, chassis-2

	n, err := BindAll(context.Background(), c, binder)
	require.NoError(t, err)
	assert.Equal(t, 6, n, "every VIF (2 switches * 3) should be bound; uplinks are not VIFs")
	assert.Len(t, *calls, 6)

	// Bindings spread round-robin across the two chassis.
	perChassis := map[string]int{}
	for _, call := range *calls {
		assert.Contains(t, strings.Join(call.args, " "), "add-port")
		perChassis[call.container]++
	}
	assert.Equal(t, 3, perChassis["clab-nw-lab-chassis-1"])
	assert.Equal(t, 3, perChassis["clab-nw-lab-chassis-2"])

	// Re-running binds nothing (idempotent): every VIF is already marked bound.
	eventuallyOwnedVIFsBound(t, c, 6)
	n, err = BindAll(context.Background(), c, binder)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestUnbindAllClearsBindings(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 1, Routers: 1, PortsPerSwitch: 2})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitchPort](t, c, 1*(2+1))

	binder, _ := newRecordingBinder()
	n, err := BindAll(context.Background(), c, binder)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	eventuallyOwnedVIFsBound(t, c, 2)

	un, err := UnbindAll(context.Background(), c, binder)
	require.NoError(t, err)
	assert.Equal(t, 2, un)
	eventuallyOwnedVIFsBound(t, c, 0)
}

func TestBindAllRequiresChassis(t *testing.T) {
	c := setupNB(t)
	_, err := BindAll(context.Background(), c, &Binder{})
	require.Error(t, err)
}

// eventuallyOwnedVIFsBound waits until exactly want owned VIFs carry a bound
// chassis marker in their external_ids.
func eventuallyOwnedVIFsBound(t *testing.T, c client.Client, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var ports []nb.LogicalSwitchPort
		if err := c.List(context.Background(), &ports); err != nil {
			return false
		}
		bound := 0
		for _, p := range ports {
			if p.ExternalIDs["nw-kind"] == "vif" && p.ExternalIDs[boundChassisKey] != "" {
				bound++
			}
		}
		return bound == want
	}, 3*time.Second, 20*time.Millisecond, "expected %d bound VIFs", want)
}

func TestAddPortBindsWhenBinderSet(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 1, PortsPerSwitch: 1})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitch](t, c, 2)

	binder, calls := newRecordingBinder()
	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 2, Routers: 1, PortsPerSwitch: 1}, Target: 2, Binder: binder})

	desc, err := sim.addPort(context.Background())
	require.NoError(t, err)
	assert.Contains(t, desc, "bound to")
	require.Len(t, *calls, 1)
	assert.Contains(t, strings.Join((*calls)[0].args, " "), "add-port")

	// Exactly the new port carries the bound-chassis marker (seeded VIFs do not).
	eventuallyOwnedVIFsBound(t, c, 1)
}

func TestCreateSwitchBindsNewVIFs(t *testing.T) {
	c := setupNB(t)
	binder, calls := newRecordingBinder()
	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 1, Routers: 1, PortsPerSwitch: 3}, Target: 3, Binder: binder})

	desc, err := sim.createSwitch(context.Background())
	require.NoError(t, err)
	assert.Contains(t, desc, "ports bound")
	require.Len(t, *calls, 3) // PortsPerSwitch
	eventuallyOwnedVIFsBound(t, c, 3)
}

func TestAddPortWithoutBinderStaysUnbound(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 1, Routers: 1, PortsPerSwitch: 1})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitch](t, c, 1)

	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 1, Routers: 1, PortsPerSwitch: 1}, Target: 1}) // no binder
	desc, err := sim.addPort(context.Background())
	require.NoError(t, err)
	assert.NotContains(t, desc, "bound to")
	eventuallyOwnedVIFsBound(t, c, 0)
}
