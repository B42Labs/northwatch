package ovnsim

import (
	"context"
	"testing"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanRemovesEverythingOwned(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 3, Routers: 2, PortsPerSwitch: 2})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitch](t, c, 3)
	eventuallyCount[nb.LoadBalancer](t, c, 2)

	n, err := Clean(context.Background(), c)
	require.NoError(t, err)
	assert.Positive(t, n, "Clean should report removed roots")

	// Root tables are emptied; their non-root children are garbage-collected.
	eventuallyCount[nb.LogicalSwitch](t, c, 0)
	eventuallyCount[nb.LogicalRouter](t, c, 0)
	eventuallyCount[nb.LoadBalancer](t, c, 0)
	eventuallyCount[nb.LogicalSwitchPort](t, c, 0)
	eventuallyCount[nb.NAT](t, c, 0)
	eventuallyCount[nb.ACL](t, c, 0)
}

func TestCleanLeavesForeignRowsUntouched(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 1, Routers: 1, PortsPerSwitch: 1})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitch](t, c, 1)

	// A switch that ovnsim does not own (no SimTag marker).
	foreign := &nb.LogicalSwitch{Name: "tenant-foreign", ExternalIDs: map[string]string{"team": "x"}}
	ops, err := c.Create(foreign)
	require.NoError(t, err)
	require.NoError(t, transact(context.Background(), c, ops))
	eventuallyCount[nb.LogicalSwitch](t, c, 2)

	_, err = Clean(context.Background(), c)
	require.NoError(t, err)

	// Only the foreign switch remains.
	eventuallyCount[nb.LogicalSwitch](t, c, 1)
	var sws []nb.LogicalSwitch
	require.NoError(t, c.List(context.Background(), &sws))
	require.Len(t, sws, 1)
	assert.Equal(t, "tenant-foreign", sws[0].Name)
}
