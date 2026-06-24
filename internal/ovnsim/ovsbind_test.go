package ovnsim

import (
	"context"
	"strings"
	"testing"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedCall struct {
	container string
	args      []string
}

func newRecordingBinder() (*Binder, *[]recordedCall) {
	var calls []recordedCall
	b := &Binder{
		LabName: "nw-lab",
		Chassis: []string{"chassis-1", "chassis-2"},
		Bridge:  "br-int",
		Run: func(_ context.Context, container string, args ...string) error {
			calls = append(calls, recordedCall{container: container, args: args})
			return nil
		},
	}
	return b, &calls
}

func TestBinderBindCommand(t *testing.T) {
	b, calls := newRecordingBinder()
	require.NoError(t, b.Bind(context.Background(), "chassis-1", "nw-ls-001-vif-001"))

	require.Len(t, *calls, 1)
	call := (*calls)[0]
	assert.Equal(t, "clab-nw-lab-chassis-1", call.container)

	joined := strings.Join(call.args, " ")
	assert.Contains(t, joined, "ovs-vsctl --may-exist add-port br-int")
	assert.Contains(t, joined, "type=internal")
	assert.Contains(t, joined, "external_ids:iface-id=nw-ls-001-vif-001")

	// OVS interface name is the short, hashed form (<= 15 chars, IFNAMSIZ).
	port := portName("nw-ls-001-vif-001")
	assert.LessOrEqual(t, len(port), 15)
	assert.Contains(t, joined, port)
}

func TestBinderUnbindCommand(t *testing.T) {
	b, calls := newRecordingBinder()
	require.NoError(t, b.Unbind(context.Background(), "chassis-2", "nw-ls-001-vif-001"))

	require.Len(t, *calls, 1)
	assert.Equal(t, "clab-nw-lab-chassis-2", (*calls)[0].container)
	joined := strings.Join((*calls)[0].args, " ")
	assert.Contains(t, joined, "ovs-vsctl --if-exists del-port br-int")
	assert.Contains(t, joined, portName("nw-ls-001-vif-001"))
}

func TestBinderMigrateDeletesThenAdds(t *testing.T) {
	b, calls := newRecordingBinder()
	require.NoError(t, b.Migrate(context.Background(), "chassis-1", "chassis-2", "nw-ls-001-vif-002"))

	require.Len(t, *calls, 2)
	// First the del-port on the source chassis...
	assert.Equal(t, "clab-nw-lab-chassis-1", (*calls)[0].container)
	assert.Contains(t, strings.Join((*calls)[0].args, " "), "del-port")
	// ...then the add-port on the destination chassis.
	assert.Equal(t, "clab-nw-lab-chassis-2", (*calls)[1].container)
	assert.Contains(t, strings.Join((*calls)[1].args, " "), "add-port")
}

func TestPortNameStableAndShort(t *testing.T) {
	a := portName("nw-ls-009-vif-042")
	bb := portName("nw-ls-009-vif-042")
	assert.Equal(t, a, bb, "portName must be deterministic")
	assert.NotEqual(t, a, portName("nw-ls-009-vif-043"))
	assert.LessOrEqual(t, len(a), 15)
	assert.True(t, strings.HasPrefix(a, "nw"))
}

// TestSimulatorBindFlowUsesBinder drives the bind action and asserts it both
// invokes the binder and records the bound chassis on the port.
func TestSimulatorBindFlowUsesBinder(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 2, Routers: 1, PortsPerSwitch: 2})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitchPort](t, c, 2*(2+1))

	binder, calls := newRecordingBinder()
	sim := NewSimulator(c, SimConfig{
		Options: Options{Switches: 2, Routers: 1, PortsPerSwitch: 2},
		Target:  2,
		Binder:  binder,
	})

	desc, err := sim.bindPort(context.Background())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(desc, "bind "), "got %q", desc)
	require.Len(t, *calls, 1)
	assert.Contains(t, strings.Join((*calls)[0].args, " "), "add-port")
}
