package ovnsim

import (
	"context"
	"fmt"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// BindAll binds every simulator-owned VIF that is not yet bound to a chassis,
// spreading them round-robin across the binder's chassis. It creates a real OVS
// interface (with the matching iface-id) on each target chassis so
// ovn-controller claims the port and fills in SB Port_Binding.chassis — which
// clears Northwatch's "VIF port not bound to any chassis" health alert.
//
// It is idempotent: VIFs already marked bound are skipped, and the underlying
// ovs-vsctl add-port uses --may-exist. Returns the number of VIFs newly bound.
func BindAll(ctx context.Context, c client.Client, binder *Binder) (int, error) {
	if binder == nil || len(binder.Chassis) == 0 {
		return 0, fmt.Errorf("no chassis configured for binding")
	}

	vifs, err := listOwned(ctx, c, func(p *nb.LogicalSwitchPort) map[string]string { return p.ExternalIDs })
	if err != nil {
		return 0, err
	}

	n := 0
	for _, p := range vifs {
		if p.ExternalIDs["nw-kind"] != "vif" || p.ExternalIDs[boundChassisKey] != "" {
			continue
		}
		chassis := binder.Chassis[n%len(binder.Chassis)]
		if err := binder.Bind(ctx, chassis, p.Name); err != nil {
			return n, fmt.Errorf("binding %s onto %s: %w", p.Name, chassis, err)
		}
		if err := recordBoundChassis(ctx, c, p.UUID, chassis); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UnbindAll removes the OVS interface for every simulator-owned VIF currently
// recorded as bound, and clears the binding marker. Returns the number unbound.
func UnbindAll(ctx context.Context, c client.Client, binder *Binder) (int, error) {
	if binder == nil {
		return 0, fmt.Errorf("no binder configured")
	}

	vifs, err := listOwned(ctx, c, func(p *nb.LogicalSwitchPort) map[string]string { return p.ExternalIDs })
	if err != nil {
		return 0, err
	}

	n := 0
	for _, p := range vifs {
		chassis := p.ExternalIDs[boundChassisKey]
		if p.ExternalIDs["nw-kind"] != "vif" || chassis == "" {
			continue
		}
		if err := binder.Unbind(ctx, chassis, p.Name); err != nil {
			return n, fmt.Errorf("unbinding %s from %s: %w", p.Name, chassis, err)
		}
		if err := recordBoundChassis(ctx, c, p.UUID, ""); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
