package correlate

import (
	"context"
	"log/slog"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// ErrNotFound is returned when the primary entity is not found in the cache.
var ErrNotFound = client.ErrNotFound

// Correlator links NB (intent) entities with SB (realization) entities.
type Correlator struct {
	NB client.Client
	SB client.Client
}

// PortBindingChain represents the full binding chain for a logical port.
type PortBindingChain struct {
	LSP          map[string]any   `json:"logical_switch_port,omitempty"`
	LRP          map[string]any   `json:"logical_router_port,omitempty"`
	PortBinding  map[string]any   `json:"port_binding,omitempty"`
	Chassis      map[string]any   `json:"chassis,omitempty"`
	Encaps       []map[string]any `json:"encaps,omitempty"`
	Datapath     map[string]any   `json:"datapath_binding,omitempty"`
	ParentSwitch map[string]any   `json:"logical_switch,omitempty"`
	ParentRouter map[string]any   `json:"logical_router,omitempty"`
}

// SwitchCorrelated is a LogicalSwitch with its SB realization.
type SwitchCorrelated struct {
	Switch   map[string]any     `json:"logical_switch"`
	Datapath map[string]any     `json:"datapath_binding,omitempty"`
	Ports    []PortBindingChain `json:"ports,omitempty"`
}

// RouterCorrelated is a LogicalRouter with its SB realization.
type RouterCorrelated struct {
	Router   map[string]any     `json:"logical_router"`
	Datapath map[string]any     `json:"datapath_binding,omitempty"`
	Ports    []PortBindingChain `json:"ports,omitempty"`
	NATs     []map[string]any   `json:"nats,omitempty"`
}

// ChassisCorrelated is a Chassis with its related entities.
type ChassisCorrelated struct {
	Chassis        map[string]any   `json:"chassis"`
	ChassisPrivate map[string]any   `json:"chassis_private,omitempty"`
	Encaps         []map[string]any `json:"encaps,omitempty"`
	PortBindings   []map[string]any `json:"port_bindings,omitempty"`
}

// SwitchSummary returns a lightweight correlation for a LogicalSwitch (switch + datapath only).
func (c *Correlator) SwitchSummary(ctx context.Context, ls nb.LogicalSwitch) SwitchCorrelated {
	result := SwitchCorrelated{
		Switch: ovndb.ModelToMap(ls),
	}
	if dp := c.findDatapathForSwitch(ctx, ls.UUID); dp != nil {
		result.Datapath = ovndb.ModelToMap(*dp)
	}
	return result
}

// SwitchDetail returns full correlation for a LogicalSwitch including all port chains.
func (c *Correlator) SwitchDetail(ctx context.Context, switchUUID string) (*SwitchCorrelated, error) {
	ls := &nb.LogicalSwitch{UUID: switchUUID}
	if err := c.NB.Get(ctx, ls); err != nil {
		return nil, err
	}

	result := &SwitchCorrelated{
		Switch: ovndb.ModelToMap(*ls),
	}

	if dp := c.findDatapathForSwitch(ctx, switchUUID); dp != nil {
		result.Datapath = ovndb.ModelToMap(*dp)
	}

	for _, portUUID := range ls.Ports {
		lsp := &nb.LogicalSwitchPort{UUID: portUUID}
		if err := c.NB.Get(ctx, lsp); err != nil {
			continue
		}
		chain := c.lspChain(ctx, *lsp)
		result.Ports = append(result.Ports, chain)
	}

	return result, nil
}

// RouterSummary returns a lightweight correlation for a LogicalRouter (router + datapath only).
func (c *Correlator) RouterSummary(ctx context.Context, lr nb.LogicalRouter) RouterCorrelated {
	result := RouterCorrelated{
		Router: ovndb.ModelToMap(lr),
	}
	if dp := c.findDatapathForRouter(ctx, lr.UUID); dp != nil {
		result.Datapath = ovndb.ModelToMap(*dp)
	}
	return result
}

// RouterDetail returns full correlation for a LogicalRouter including port chains and NATs.
func (c *Correlator) RouterDetail(ctx context.Context, routerUUID string) (*RouterCorrelated, error) {
	lr := &nb.LogicalRouter{UUID: routerUUID}
	if err := c.NB.Get(ctx, lr); err != nil {
		return nil, err
	}

	result := &RouterCorrelated{
		Router: ovndb.ModelToMap(*lr),
	}

	if dp := c.findDatapathForRouter(ctx, routerUUID); dp != nil {
		result.Datapath = ovndb.ModelToMap(*dp)
	}

	for _, portUUID := range lr.Ports {
		lrp := &nb.LogicalRouterPort{UUID: portUUID}
		if err := c.NB.Get(ctx, lrp); err != nil {
			continue
		}
		chain := c.lrpChain(ctx, *lrp)
		result.Ports = append(result.Ports, chain)
	}

	for _, natUUID := range lr.Nat {
		nat := &nb.NAT{UUID: natUUID}
		if err := c.NB.Get(ctx, nat); err != nil {
			continue
		}
		result.NATs = append(result.NATs, ovndb.ModelToMap(*nat))
	}

	return result, nil
}

// LSPDetail returns the full PortBindingChain for a LogicalSwitchPort.
func (c *Correlator) LSPDetail(ctx context.Context, lspUUID string) (*PortBindingChain, error) {
	lsp := &nb.LogicalSwitchPort{UUID: lspUUID}
	if err := c.NB.Get(ctx, lsp); err != nil {
		return nil, err
	}

	chain := c.lspChain(ctx, *lsp)

	// Find parent switch
	var switches []nb.LogicalSwitch
	if err := c.NB.WhereCache(func(ls *nb.LogicalSwitch) bool {
		for _, p := range ls.Ports {
			if p == lspUUID {
				return true
			}
		}
		return false
	}).List(ctx, &switches); err != nil {
		slog.Error("correlate listing switches for LSP failed", "lsp", lspUUID, "err", err)
	}
	if len(switches) > 0 {
		chain.ParentSwitch = ovndb.ModelToMap(switches[0])
	}

	return &chain, nil
}

// LRPDetail returns the full PortBindingChain for a LogicalRouterPort.
func (c *Correlator) LRPDetail(ctx context.Context, lrpUUID string) (*PortBindingChain, error) {
	lrp := &nb.LogicalRouterPort{UUID: lrpUUID}
	if err := c.NB.Get(ctx, lrp); err != nil {
		return nil, err
	}

	chain := c.lrpChain(ctx, *lrp)

	// Find parent router
	var routers []nb.LogicalRouter
	if err := c.NB.WhereCache(func(lr *nb.LogicalRouter) bool {
		for _, p := range lr.Ports {
			if p == lrpUUID {
				return true
			}
		}
		return false
	}).List(ctx, &routers); err != nil {
		slog.Error("correlate listing routers for LRP failed", "lrp", lrpUUID, "err", err)
	}
	if len(routers) > 0 {
		chain.ParentRouter = ovndb.ModelToMap(routers[0])
	}

	return &chain, nil
}

// ChassisSummary returns a lightweight correlation for a Chassis (chassis + encaps).
func (c *Correlator) ChassisSummary(ctx context.Context, ch sb.Chassis) ChassisCorrelated {
	result := ChassisCorrelated{
		Chassis: ovndb.ModelToMap(ch),
	}
	for _, encapUUID := range ch.Encaps {
		enc := &sb.Encap{UUID: encapUUID}
		if c.SB.Get(ctx, enc) == nil {
			result.Encaps = append(result.Encaps, ovndb.ModelToMap(*enc))
		}
	}
	return result
}

// ChassisDetail returns full correlation for a Chassis including private info and hosted ports.
func (c *Correlator) ChassisDetail(ctx context.Context, chassisUUID string) (*ChassisCorrelated, error) {
	ch := &sb.Chassis{UUID: chassisUUID}
	if err := c.SB.Get(ctx, ch); err != nil {
		return nil, err
	}

	result := &ChassisCorrelated{
		Chassis: ovndb.ModelToMap(*ch),
	}

	// Find ChassisPrivate by name
	var cps []sb.ChassisPrivate
	if err := c.SB.WhereCache(func(cp *sb.ChassisPrivate) bool {
		return cp.Name == ch.Name
	}).List(ctx, &cps); err != nil {
		slog.Error("correlate listing ChassisPrivate failed", "chassis", ch.Name, "err", err)
	}
	if len(cps) > 0 {
		result.ChassisPrivate = ovndb.ModelToMap(cps[0])
	}

	// Resolve encaps
	for _, encapUUID := range ch.Encaps {
		enc := &sb.Encap{UUID: encapUUID}
		if c.SB.Get(ctx, enc) == nil {
			result.Encaps = append(result.Encaps, ovndb.ModelToMap(*enc))
		}
	}

	// Find all PortBindings hosted on this chassis
	var pbs []sb.PortBinding
	if err := c.SB.WhereCache(func(pb *sb.PortBinding) bool {
		return pb.Chassis != nil && *pb.Chassis == chassisUUID
	}).List(ctx, &pbs); err != nil {
		slog.Error("correlate listing PortBindings for chassis failed", "chassis", chassisUUID, "err", err)
	}
	for _, pb := range pbs {
		result.PortBindings = append(result.PortBindings, ovndb.ModelToMap(pb))
	}

	return result, nil
}

// PortBindingDetail returns a PortBinding with its NB source and Chassis resolved.
func (c *Correlator) PortBindingDetail(ctx context.Context, pbUUID string) (*PortBindingChain, error) {
	pb := &sb.PortBinding{UUID: pbUUID}
	if err := c.SB.Get(ctx, pb); err != nil {
		return nil, err
	}

	chain := PortBindingChain{
		PortBinding: ovndb.ModelToMap(*pb),
	}

	c.resolveBindingDetails(ctx, pb, &chain)

	// Resolve NB source by logical port name
	var lsps []nb.LogicalSwitchPort
	if err := c.NB.WhereCache(func(lsp *nb.LogicalSwitchPort) bool {
		return lsp.Name == pb.LogicalPort
	}).List(ctx, &lsps); err != nil {
		slog.Error("correlate listing LSPs for port failed", "port", pb.LogicalPort, "err", err)
	}
	if len(lsps) > 0 {
		chain.LSP = ovndb.ModelToMap(lsps[0])
		// Find parent switch
		lspUUID := lsps[0].UUID
		var switches []nb.LogicalSwitch
		if err := c.NB.WhereCache(func(ls *nb.LogicalSwitch) bool {
			for _, p := range ls.Ports {
				if p == lspUUID {
					return true
				}
			}
			return false
		}).List(ctx, &switches); err != nil {
			slog.Error("correlate listing switches for LSP failed", "lsp", lspUUID, "err", err)
		}
		if len(switches) > 0 {
			chain.ParentSwitch = ovndb.ModelToMap(switches[0])
		}
	} else {
		// Try as LRP
		var lrps []nb.LogicalRouterPort
		if err := c.NB.WhereCache(func(lrp *nb.LogicalRouterPort) bool {
			return lrp.Name == pb.LogicalPort
		}).List(ctx, &lrps); err != nil {
			slog.Error("correlate listing LRPs for port failed", "port", pb.LogicalPort, "err", err)
		}
		if len(lrps) > 0 {
			chain.LRP = ovndb.ModelToMap(lrps[0])
			lrpUUID := lrps[0].UUID
			var routers []nb.LogicalRouter
			if err := c.NB.WhereCache(func(lr *nb.LogicalRouter) bool {
				for _, p := range lr.Ports {
					if p == lrpUUID {
						return true
					}
				}
				return false
			}).List(ctx, &routers); err != nil {
				slog.Error("correlate listing routers for LRP failed", "lrp", lrpUUID, "err", err)
			}
			if len(routers) > 0 {
				chain.ParentRouter = ovndb.ModelToMap(routers[0])
			}
		}
	}

	return &chain, nil
}

// lspChain builds a PortBindingChain for a LogicalSwitchPort.
func (c *Correlator) lspChain(ctx context.Context, lsp nb.LogicalSwitchPort) PortBindingChain {
	chain := PortBindingChain{
		LSP: ovndb.ModelToMap(lsp),
	}

	pb := c.findPortBinding(ctx, lsp.Name)
	if pb == nil {
		return chain
	}
	chain.PortBinding = ovndb.ModelToMap(*pb)
	c.resolveBindingDetails(ctx, pb, &chain)
	return chain
}

// lrpChain builds a PortBindingChain for a LogicalRouterPort.
// It tries both the direct name and the "cr-" prefix for gateway redirect ports.
func (c *Correlator) lrpChain(ctx context.Context, lrp nb.LogicalRouterPort) PortBindingChain {
	chain := PortBindingChain{
		LRP: ovndb.ModelToMap(lrp),
	}

	pb := c.findPortBinding(ctx, lrp.Name)
	if pb == nil {
		pb = c.findPortBinding(ctx, "cr-"+lrp.Name)
	}
	if pb == nil {
		return chain
	}
	chain.PortBinding = ovndb.ModelToMap(*pb)
	c.resolveBindingDetails(ctx, pb, &chain)
	return chain
}

// resolveBindingDetails fills in Chassis, Encaps, and Datapath for a PortBinding.
func (c *Correlator) resolveBindingDetails(ctx context.Context, pb *sb.PortBinding, chain *PortBindingChain) {
	if pb.Chassis != nil && *pb.Chassis != "" {
		ch := &sb.Chassis{UUID: *pb.Chassis}
		if c.SB.Get(ctx, ch) == nil {
			chain.Chassis = ovndb.ModelToMap(*ch)
			for _, encapUUID := range ch.Encaps {
				enc := &sb.Encap{UUID: encapUUID}
				if c.SB.Get(ctx, enc) == nil {
					chain.Encaps = append(chain.Encaps, ovndb.ModelToMap(*enc))
				}
			}
		}
	}

	dp := &sb.DatapathBinding{UUID: pb.Datapath}
	if c.SB.Get(ctx, dp) == nil {
		chain.Datapath = ovndb.ModelToMap(*dp)
	}
}

func (c *Correlator) findPortBinding(ctx context.Context, logicalPort string) *sb.PortBinding {
	var pbs []sb.PortBinding
	if err := c.SB.WhereCache(func(pb *sb.PortBinding) bool {
		return pb.LogicalPort == logicalPort
	}).List(ctx, &pbs); err != nil {
		slog.Error("correlate finding PortBinding failed", "port", logicalPort, "err", err)
		return nil
	}
	if len(pbs) == 0 {
		return nil
	}
	return &pbs[0]
}

func (c *Correlator) findDatapathForSwitch(ctx context.Context, switchUUID string) *sb.DatapathBinding {
	var dps []sb.DatapathBinding
	if err := c.SB.WhereCache(func(dp *sb.DatapathBinding) bool {
		return dp.ExternalIDs["logical-switch"] == switchUUID
	}).List(ctx, &dps); err != nil {
		slog.Error("correlate finding DatapathBinding for switch failed", "switch", switchUUID, "err", err)
		return nil
	}
	if len(dps) == 0 {
		return nil
	}
	return &dps[0]
}

func (c *Correlator) findDatapathForRouter(ctx context.Context, routerUUID string) *sb.DatapathBinding {
	var dps []sb.DatapathBinding
	if err := c.SB.WhereCache(func(dp *sb.DatapathBinding) bool {
		return dp.ExternalIDs["logical-router"] == routerUUID
	}).List(ctx, &dps); err != nil {
		slog.Error("correlate finding DatapathBinding for router failed", "router", routerUUID, "err", err)
		return nil
	}
	if len(dps) == 0 {
		return nil
	}
	return &dps[0]
}
