package snapshot_test

import (
	"bytes"
	"context"
	"testing"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/snapshot"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoundTrip captures two populated in-memory databases, serializes them to a
// file, serves them back offline, and verifies the data — including cross-table
// references — survives intact with original UUIDs preserved.
func TestRoundTrip(t *testing.T) {
	srcNB := testutil.SetupNBTestClient(t)
	srcSB := testutil.SetupSBTestClient(t)

	// NB: a switch plus a gateway router whose port and NAT are non-root rows
	// reachable only via the router — a strict reference-integrity check.
	testutil.InsertNBGlobal(t, srcNB, 7, 7, 7)
	testutil.InsertLogicalSwitch(t, srcNB, "ls0")
	testutil.InsertGatewayRouter(t, srcNB, "gw0", "lrp0", []string{"10.0.0.1/24"}, []string{"1.2.3.4"})

	// SB: a chassis and a port binding that references it.
	chUUID := testutil.InsertChassis(t, srcSB, "ch0", "host0", "10.0.0.5")
	testutil.InsertPortBinding(t, srcSB, "lsp0", "", &chUUID)

	// Capture -> Save -> Load (exercise the on-disk JSON format too).
	snap, err := snapshot.Capture(srcNB, srcSB, nb.Schema().Name, sb.Schema().Name)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, snapshot.Save(snap, &buf))
	loaded, err := snapshot.Load(&buf)
	require.NoError(t, err)

	// Serve the offline copy and connect exactly like production does.
	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	servers, err := snapshot.Serve(loaded, nbModel, sbModel, nb.Schema(), sb.Schema())
	require.NoError(t, err)
	defer servers.Close()

	ctx := context.Background()
	dbs, err := ovndb.Connect(ctx, servers.NBAddr, servers.SBAddr, nbModel, sbModel)
	require.NoError(t, err)
	defer dbs.Close()

	// NB_Global survived with its config generations.
	var globals []nb.NBGlobal
	require.NoError(t, dbs.NB.List(ctx, &globals))
	require.Len(t, globals, 1)
	assert.Equal(t, 7, globals[0].NbCfg)

	// Logical switch survived.
	var switches []nb.LogicalSwitch
	require.NoError(t, dbs.NB.List(ctx, &switches))
	require.Len(t, switches, 1)
	assert.Equal(t, "ls0", switches[0].Name)

	// Router survived and still references its port by the original UUID.
	var routers []nb.LogicalRouter
	require.NoError(t, dbs.NB.List(ctx, &routers))
	require.Len(t, routers, 1)
	assert.Equal(t, "gw0", routers[0].Name)
	require.Len(t, routers[0].Ports, 1, "router port reference must be preserved")
	require.Len(t, routers[0].Nat, 1, "router NAT reference must be preserved")

	lrpByUUID := map[string]nb.LogicalRouterPort{}
	var lrps []nb.LogicalRouterPort
	require.NoError(t, dbs.NB.List(ctx, &lrps))
	for _, p := range lrps {
		lrpByUUID[p.UUID] = p
	}
	port, ok := lrpByUUID[routers[0].Ports[0]]
	require.True(t, ok, "referenced router port UUID must resolve to an existing row")
	assert.Equal(t, "lrp0", port.Name)

	// SB chassis survived with its original UUID, and the port binding still
	// points at it.
	var chassis []sb.Chassis
	require.NoError(t, dbs.SB.List(ctx, &chassis))
	require.Len(t, chassis, 1)
	assert.Equal(t, "ch0", chassis[0].Name)
	assert.Equal(t, chUUID, chassis[0].UUID, "chassis UUID must be preserved exactly")

	var bindings []sb.PortBinding
	require.NoError(t, dbs.SB.List(ctx, &bindings))
	require.Len(t, bindings, 1)
	assert.Equal(t, "lsp0", bindings[0].LogicalPort)
	require.NotNil(t, bindings[0].Chassis)
	assert.Equal(t, chUUID, *bindings[0].Chassis, "port binding must still reference the chassis")
}

// TestLoadRejectsWrongVersion ensures the loader guards the format version.
func TestLoadRejectsWrongVersion(t *testing.T) {
	_, err := snapshot.Load(bytes.NewBufferString(`{"version": 999}`))
	require.Error(t, err)
}
