package debug

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// CheckStatus represents the result status of a connectivity check.
type CheckStatus string

const (
	StatusPass    CheckStatus = "pass"
	StatusFail    CheckStatus = "fail"
	StatusWarning CheckStatus = "warning"
	StatusSkipped CheckStatus = "skipped"
)

// ConnectivityCheck is a single check result.
type ConnectivityCheck struct {
	Name     string      `json:"name"`
	Category string      `json:"category"`
	Status   CheckStatus `json:"status"`
	Message  string      `json:"message"`
	Details  any         `json:"details,omitempty"`
}

// PortInfo summarizes a port for the connectivity result.
type PortInfo struct {
	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	Type         string   `json:"type,omitempty"`
	SwitchName   string   `json:"switch_name,omitempty"`
	BoundChassis string   `json:"bound_chassis,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
}

// ConnectivityResult is the full result of a connectivity check.
type ConnectivityResult struct {
	Source      PortInfo            `json:"source"`
	Destination PortInfo           `json:"destination"`
	Overall     CheckStatus        `json:"overall"`
	Checks      []ConnectivityCheck `json:"checks"`
}

// ConnectivityChecker analyzes connectivity between two logical ports.
type ConnectivityChecker struct {
	NB client.Client
	SB client.Client
}

// Check performs ordered connectivity checks between source and destination ports.
func (c *ConnectivityChecker) Check(ctx context.Context, srcUUID, dstUUID string) (*ConnectivityResult, error) {
	result := &ConnectivityResult{
		Overall: StatusPass,
	}

	// 1. Port resolution
	srcLSP, srcSwitch, srcPB, err := c.resolvePort(ctx, srcUUID)
	if err != nil {
		result.addCheck(ConnectivityCheck{
			Name:     "source_resolution",
			Category: "resolution",
			Status:   StatusFail,
			Message:  fmt.Sprintf("Source port not found: %v", err),
		})
		result.Overall = StatusFail
		return result, nil
	}
	result.Source = c.buildPortInfo(srcLSP, srcSwitch, srcPB)
	result.addCheck(ConnectivityCheck{
		Name:     "source_resolution",
		Category: "resolution",
		Status:   StatusPass,
		Message:  fmt.Sprintf("Source port %q resolved on switch %q", srcLSP.Name, switchName(srcSwitch)),
	})

	dstLSP, dstSwitch, dstPB, err := c.resolvePort(ctx, dstUUID)
	if err != nil {
		result.addCheck(ConnectivityCheck{
			Name:     "destination_resolution",
			Category: "resolution",
			Status:   StatusFail,
			Message:  fmt.Sprintf("Destination port not found: %v", err),
		})
		result.Overall = StatusFail
		return result, nil
	}
	result.Destination = c.buildPortInfo(dstLSP, dstSwitch, dstPB)
	result.addCheck(ConnectivityCheck{
		Name:     "destination_resolution",
		Category: "resolution",
		Status:   StatusPass,
		Message:  fmt.Sprintf("Destination port %q resolved on switch %q", dstLSP.Name, switchName(dstSwitch)),
	})

	// 2. L2 connectivity + 3. L3 connectivity
	sameSwitch := srcSwitch != nil && dstSwitch != nil && srcSwitch.UUID == dstSwitch.UUID
	if sameSwitch {
		result.addCheck(ConnectivityCheck{
			Name:     "l2_connectivity",
			Category: "l2",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Both ports on same switch %q", srcSwitch.Name),
		})
		result.addCheck(ConnectivityCheck{
			Name:     "l3_connectivity",
			Category: "l3",
			Status:   StatusSkipped,
			Message:  "Same-switch — L3 check skipped",
		})
	} else {
		// Check for cross-switch via router
		router := c.findConnectingRouter(ctx, srcSwitch, dstSwitch)
		if router != nil {
			result.addCheck(ConnectivityCheck{
				Name:     "l2_connectivity",
				Category: "l2",
				Status:   StatusPass,
				Message:  fmt.Sprintf("Cross-switch connectivity via router %q", router.Name),
			})
			result.addCheck(c.checkL3Connectivity(ctx, router.UUID))
		} else {
			result.addCheck(ConnectivityCheck{
				Name:     "l2_connectivity",
				Category: "l2",
				Status:   StatusFail,
				Message:  "Ports are on different switches with no connecting router found",
			})
			result.addCheck(ConnectivityCheck{
				Name:     "l3_connectivity",
				Category: "l3",
				Status:   StatusSkipped,
				Message:  "No connecting router — L3 check skipped",
			})
		}
	}

	// 4. ACL analysis
	result.addCheck(c.checkACLs(ctx, srcLSP, dstLSP, srcSwitch, dstSwitch))

	// 5. Physical realization
	result.addCheck(c.checkPhysicalRealization(ctx, srcLSP, srcPB, "source"))
	result.addCheck(c.checkPhysicalRealization(ctx, dstLSP, dstPB, "destination"))

	if srcPB != nil && dstPB != nil &&
		srcPB.Chassis != nil && *srcPB.Chassis != "" &&
		dstPB.Chassis != nil && *dstPB.Chassis != "" &&
		*srcPB.Chassis != *dstPB.Chassis {
		result.addCheck(c.checkEncapTunnel(ctx, *srcPB.Chassis, *dstPB.Chassis))
	}

	return result, nil
}

func (result *ConnectivityResult) addCheck(check ConnectivityCheck) {
	result.Checks = append(result.Checks, check)
	if check.Status == StatusFail {
		result.Overall = StatusFail
	} else if check.Status == StatusWarning && result.Overall == StatusPass {
		result.Overall = StatusWarning
	}
}

func switchName(ls *nb.LogicalSwitch) string {
	if ls == nil {
		return "<unknown>"
	}
	return ls.Name
}

func (c *ConnectivityChecker) resolvePort(ctx context.Context, uuid string) (*nb.LogicalSwitchPort, *nb.LogicalSwitch, *sb.PortBinding, error) {
	lsp := &nb.LogicalSwitchPort{UUID: uuid}
	if err := c.NB.Get(ctx, lsp); err != nil {
		return nil, nil, nil, err
	}

	// Find parent switch
	var switches []nb.LogicalSwitch
	_ = c.NB.WhereCache(func(ls *nb.LogicalSwitch) bool {
		for _, p := range ls.Ports {
			if p == uuid {
				return true
			}
		}
		return false
	}).List(ctx, &switches)

	var parentSwitch *nb.LogicalSwitch
	if len(switches) > 0 {
		parentSwitch = &switches[0]
	}

	// Find port binding
	var pbs []sb.PortBinding
	_ = c.SB.WhereCache(func(pb *sb.PortBinding) bool {
		return pb.LogicalPort == lsp.Name
	}).List(ctx, &pbs)

	var pb *sb.PortBinding
	if len(pbs) > 0 {
		pb = &pbs[0]
	}

	return lsp, parentSwitch, pb, nil
}

func (c *ConnectivityChecker) buildPortInfo(lsp *nb.LogicalSwitchPort, ls *nb.LogicalSwitch, pb *sb.PortBinding) PortInfo {
	info := PortInfo{
		UUID:      lsp.UUID,
		Name:      lsp.Name,
		Type:      lsp.Type,
		Addresses: lsp.Addresses,
	}
	if ls != nil {
		info.SwitchName = ls.Name
	}
	if pb != nil && pb.Chassis != nil {
		info.BoundChassis = *pb.Chassis
	}
	return info
}

func (c *ConnectivityChecker) findConnectingRouter(ctx context.Context, srcSwitch, dstSwitch *nb.LogicalSwitch) *nb.LogicalRouter {
	if srcSwitch == nil || dstSwitch == nil {
		return nil
	}

	// Find router ports on each switch
	srcRouterPorts := c.findRouterPortsOnSwitch(ctx, srcSwitch)
	dstRouterPorts := c.findRouterPortsOnSwitch(ctx, dstSwitch)

	// Check if any router port pair belongs to the same router
	for _, srcLRP := range srcRouterPorts {
		srcRouter := c.findParentRouter(ctx, srcLRP)
		if srcRouter == nil {
			continue
		}
		for _, dstLRP := range dstRouterPorts {
			dstRouter := c.findParentRouter(ctx, dstLRP)
			if dstRouter != nil && srcRouter.UUID == dstRouter.UUID {
				return srcRouter
			}
		}
	}
	return nil
}

func (c *ConnectivityChecker) findRouterPortsOnSwitch(ctx context.Context, ls *nb.LogicalSwitch) []string {
	var routerPorts []string
	for _, portUUID := range ls.Ports {
		lsp := &nb.LogicalSwitchPort{UUID: portUUID}
		if err := c.NB.Get(ctx, lsp); err != nil {
			continue
		}
		if lsp.Type == "router" {
			if rp, ok := lsp.Options["router-port"]; ok {
				routerPorts = append(routerPorts, rp)
			}
		}
	}
	return routerPorts
}

func (c *ConnectivityChecker) findParentRouter(ctx context.Context, lrpName string) *nb.LogicalRouter {
	var lrps []nb.LogicalRouterPort
	_ = c.NB.WhereCache(func(lrp *nb.LogicalRouterPort) bool {
		return lrp.Name == lrpName
	}).List(ctx, &lrps)
	if len(lrps) == 0 {
		return nil
	}

	var routers []nb.LogicalRouter
	lrpUUID := lrps[0].UUID
	_ = c.NB.WhereCache(func(lr *nb.LogicalRouter) bool {
		for _, p := range lr.Ports {
			if p == lrpUUID {
				return true
			}
		}
		return false
	}).List(ctx, &routers)
	if len(routers) > 0 {
		return &routers[0]
	}
	return nil
}

func (c *ConnectivityChecker) checkL3Connectivity(ctx context.Context, routerUUID string) ConnectivityCheck {
	lr := &nb.LogicalRouter{UUID: routerUUID}
	if err := c.NB.Get(ctx, lr); err != nil {
		return ConnectivityCheck{
			Name:     "l3_connectivity",
			Category: "l3",
			Status:   StatusWarning,
			Message:  "Could not verify L3 connectivity — router not found",
		}
	}

	// Check for static routes
	hasRoutes := len(lr.StaticRoutes) > 0
	hasNAT := len(lr.Nat) > 0

	msg := fmt.Sprintf("Router %q has %d ports", lr.Name, len(lr.Ports))
	if hasRoutes {
		msg += fmt.Sprintf(", %d static routes", len(lr.StaticRoutes))
	}
	if hasNAT {
		msg += fmt.Sprintf(", %d NAT rules", len(lr.Nat))
	}

	return ConnectivityCheck{
		Name:     "l3_connectivity",
		Category: "l3",
		Status:   StatusPass,
		Message:  msg,
	}
}

func (c *ConnectivityChecker) checkACLs(ctx context.Context, srcLSP, dstLSP *nb.LogicalSwitchPort, srcSwitch, dstSwitch *nb.LogicalSwitch) ConnectivityCheck {
	// Collect ACLs from both switches
	var aclUUIDs []string
	if srcSwitch != nil {
		aclUUIDs = append(aclUUIDs, srcSwitch.ACLs...)
	}
	if dstSwitch != nil && (srcSwitch == nil || srcSwitch.UUID != dstSwitch.UUID) {
		aclUUIDs = append(aclUUIDs, dstSwitch.ACLs...)
	}

	// Also check port groups containing these ports
	var portGroups []nb.PortGroup
	_ = c.NB.WhereCache(func(pg *nb.PortGroup) bool {
		for _, p := range pg.Ports {
			if p == srcLSP.UUID || p == dstLSP.UUID {
				return true
			}
		}
		return false
	}).List(ctx, &portGroups)

	for _, pg := range portGroups {
		aclUUIDs = append(aclUUIDs, pg.ACLs...)
	}

	if len(aclUUIDs) == 0 {
		return ConnectivityCheck{
			Name:     "acl_analysis",
			Category: "acl",
			Status:   StatusPass,
			Message:  "No ACLs found on switches or port groups",
		}
	}

	// Check for drop/reject rules that reference these ports
	var denyCount int
	for _, aclUUID := range aclUUIDs {
		acl := &nb.ACL{UUID: aclUUID}
		if err := c.NB.Get(ctx, acl); err != nil {
			continue
		}
		if acl.Action == "drop" || acl.Action == "reject" {
			matchLower := strings.ToLower(acl.Match)
			if strings.Contains(matchLower, strings.ToLower(srcLSP.Name)) ||
				strings.Contains(matchLower, strings.ToLower(dstLSP.Name)) {
				denyCount++
			}
		}
	}

	if denyCount > 0 {
		return ConnectivityCheck{
			Name:     "acl_analysis",
			Category: "acl",
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Found %d drop/reject ACL(s) referencing these ports (heuristic match)", denyCount),
		}
	}

	return ConnectivityCheck{
		Name:     "acl_analysis",
		Category: "acl",
		Status:   StatusPass,
		Message:  fmt.Sprintf("Checked %d ACLs — no drop/reject rules matching these ports", len(aclUUIDs)),
	}
}

func (c *ConnectivityChecker) checkPhysicalRealization(ctx context.Context, lsp *nb.LogicalSwitchPort, pb *sb.PortBinding, label string) ConnectivityCheck {
	// Non-VIF types don't need physical binding
	if lsp.Type != "" {
		return ConnectivityCheck{
			Name:     label + "_physical",
			Category: "physical",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Port type %q does not require chassis binding", lsp.Type),
		}
	}

	if pb == nil {
		return ConnectivityCheck{
			Name:     label + "_physical",
			Category: "physical",
			Status:   StatusFail,
			Message:  fmt.Sprintf("%s port has no port binding", capitalize(label)),
		}
	}

	if pb.Chassis == nil || *pb.Chassis == "" {
		return ConnectivityCheck{
			Name:     label + "_physical",
			Category: "physical",
			Status:   StatusFail,
			Message:  fmt.Sprintf("%s port is not bound to a chassis", capitalize(label)),
		}
	}

	if pb.Up != nil && !*pb.Up {
		return ConnectivityCheck{
			Name:     label + "_physical",
			Category: "physical",
			Status:   StatusWarning,
			Message:  fmt.Sprintf("%s port binding is down", capitalize(label)),
		}
	}

	return ConnectivityCheck{
		Name:     label + "_physical",
		Category: "physical",
		Status:   StatusPass,
		Message:  fmt.Sprintf("%s port bound and up", capitalize(label)),
	}
}

func (c *ConnectivityChecker) checkEncapTunnel(ctx context.Context, srcChassisUUID, dstChassisUUID string) ConnectivityCheck {
	srcChassis := &sb.Chassis{UUID: srcChassisUUID}
	if err := c.SB.Get(ctx, srcChassis); err != nil {
		return ConnectivityCheck{
			Name:     "encap_tunnel",
			Category: "physical",
			Status:   StatusWarning,
			Message:  "Could not verify tunnel — source chassis not found",
		}
	}

	dstChassis := &sb.Chassis{UUID: dstChassisUUID}
	if err := c.SB.Get(ctx, dstChassis); err != nil {
		return ConnectivityCheck{
			Name:     "encap_tunnel",
			Category: "physical",
			Status:   StatusWarning,
			Message:  "Could not verify tunnel — destination chassis not found",
		}
	}

	if len(srcChassis.Encaps) > 0 && len(dstChassis.Encaps) > 0 {
		return ConnectivityCheck{
			Name:     "encap_tunnel",
			Category: "physical",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Tunnel encaps exist between chassis %s and %s", srcChassis.Name, dstChassis.Name),
		}
	}

	return ConnectivityCheck{
		Name:     "encap_tunnel",
		Category: "physical",
		Status:   StatusWarning,
		Message:  "One or both chassis missing encap configuration",
	}
}
