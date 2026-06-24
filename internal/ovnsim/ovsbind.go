package ovnsim

import (
	"context"
	"fmt"
	"hash/fnv"
	"os/exec"
	"strings"
)

// Binder creates and moves OVS interfaces on the lab chassis containers so that
// logical ports actually bind to a chassis. ovn-controller then claims the port
// and fills in SB Port_Binding.chassis, making port/chassis movement visible in
// Northwatch. It drives the chassis OVS via `docker exec clab-<lab>-<chassis>
// ovs-vsctl ...`, mirroring how the upstream e2e bootstrap wires a workload.
//
// The Run hook is injectable so tests can assert the issued commands without a
// real Docker daemon.
type Binder struct {
	LabName string
	Chassis []string
	Bridge  string
	// Run executes a command inside the named container. Defaults to docker exec.
	Run func(ctx context.Context, container string, args ...string) error
}

// NewBinder returns a Binder targeting the given containerlab lab and chassis,
// using `docker exec` by default.
func NewBinder(labName string, chassis []string) *Binder {
	return &Binder{
		LabName: labName,
		Chassis: chassis,
		Bridge:  "br-int",
		Run:     dockerExec,
	}
}

// container returns the containerlab container name for a chassis node.
func (b *Binder) container(chassis string) string {
	return fmt.Sprintf("clab-%s-%s", b.LabName, chassis)
}

// portName derives a short, deterministic OVS port name from the logical port
// id. OVS interface names are capped at 15 chars (IFNAMSIZ), so the full
// logical-port name cannot be used directly; the iface-id external_id carries
// the real binding key.
func portName(ifaceID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ifaceID))
	return fmt.Sprintf("nw%08x", h.Sum32())
}

// Bind creates an internal OVS port on the chassis bound to the logical port
// via external_ids:iface-id.
func (b *Binder) Bind(ctx context.Context, chassis, ifaceID string) error {
	port := portName(ifaceID)
	return b.Run(ctx, b.container(chassis),
		"ovs-vsctl", "--may-exist", "add-port", b.Bridge, port,
		"--", "set", "interface", port, "type=internal",
		"external_ids:iface-id="+ifaceID,
	)
}

// Unbind deletes the OVS port for a logical port from the chassis.
func (b *Binder) Unbind(ctx context.Context, chassis, ifaceID string) error {
	port := portName(ifaceID)
	return b.Run(ctx, b.container(chassis),
		"ovs-vsctl", "--if-exists", "del-port", b.Bridge, port,
	)
}

// Migrate moves a logical port's binding from one chassis to another by
// deleting the OVS port on the source and recreating it on the destination.
func (b *Binder) Migrate(ctx context.Context, from, to, ifaceID string) error {
	if err := b.Unbind(ctx, from, ifaceID); err != nil {
		return fmt.Errorf("unbind from %s: %w", from, err)
	}
	if err := b.Bind(ctx, to, ifaceID); err != nil {
		return fmt.Errorf("bind to %s: %w", to, err)
	}
	return nil
}

// dockerExec runs `docker exec <container> <args...>`.
func dockerExec(ctx context.Context, container string, args ...string) error {
	full := append([]string{"exec", container}, args...)
	// docker is a fixed binary; the arguments are built from controlled lab and
	// chassis identifiers and ovs-vsctl literals, not user free-text.
	cmd := exec.CommandContext(ctx, "docker", full...) //nolint:gosec // G204: controlled args, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w: %s", strings.Join(full, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
