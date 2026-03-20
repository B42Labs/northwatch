package debug

import (
	"context"
	"fmt"
	"sort"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// DiagnosticSeverity represents the health status of a check.
type DiagnosticSeverity string

const (
	SeverityHealthy DiagnosticSeverity = "healthy"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityError   DiagnosticSeverity = "error"
)

// DiagnosticCheck is a single diagnostic result for a port.
type DiagnosticCheck struct {
	Name    string             `json:"name"`
	Status  DiagnosticSeverity `json:"status"`
	Message string             `json:"message"`
}

// PortDiagnostic is the full diagnostic result for a single port.
type PortDiagnostic struct {
	PortUUID   string             `json:"port_uuid"`
	PortName   string             `json:"port_name"`
	PortType   string             `json:"port_type"`
	SwitchName string             `json:"switch_name,omitempty"`
	Overall    DiagnosticSeverity `json:"overall"`
	Checks     []DiagnosticCheck  `json:"checks"`
}

// PortDiagnosticsSummary aggregates diagnostics across all ports.
type PortDiagnosticsSummary struct {
	Total   int              `json:"total"`
	Healthy int              `json:"healthy"`
	Warning int              `json:"warning"`
	Error   int              `json:"error"`
	Ports   []PortDiagnostic `json:"ports"`
}

// PortDiagnoser analyzes port health using NB and SB caches.
type PortDiagnoser struct {
	NB client.Client
	SB client.Client
}

// DiagnosePort runs all diagnostic checks on a single port identified by UUID.
func (d *PortDiagnoser) DiagnosePort(ctx context.Context, portUUID string) (*PortDiagnostic, error) {
	lsp := &nb.LogicalSwitchPort{UUID: portUUID}
	if err := d.NB.Get(ctx, lsp); err != nil {
		return nil, fmt.Errorf("port not found: %w", err)
	}

	switchName := d.findParentSwitchName(ctx, portUUID)
	return d.diagnose(ctx, *lsp, switchName), nil
}

// DiagnoseAll runs diagnostics on all logical switch ports and returns an aggregate summary.
func (d *PortDiagnoser) DiagnoseAll(ctx context.Context) (*PortDiagnosticsSummary, error) {
	var lsps []nb.LogicalSwitchPort
	if err := d.NB.List(ctx, &lsps); err != nil {
		return nil, fmt.Errorf("listing logical switch ports: %w", err)
	}

	// Build switch name lookup
	switchNames := d.buildSwitchNameMap(ctx)

	summary := &PortDiagnosticsSummary{
		Total: len(lsps),
		Ports: make([]PortDiagnostic, 0, len(lsps)),
	}

	for _, lsp := range lsps {
		switchName := switchNames[lsp.UUID]
		diag := d.diagnose(ctx, lsp, switchName)
		switch diag.Overall {
		case SeverityHealthy:
			summary.Healthy++
		case SeverityWarning:
			summary.Warning++
		case SeverityError:
			summary.Error++
		}
		summary.Ports = append(summary.Ports, *diag)
	}

	// Sort: errors first, then warnings, then healthy
	sort.Slice(summary.Ports, func(i, j int) bool {
		return severityOrder(summary.Ports[i].Overall) < severityOrder(summary.Ports[j].Overall)
	})

	return summary, nil
}

func severityOrder(s DiagnosticSeverity) int {
	switch s {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

func (d *PortDiagnoser) diagnose(ctx context.Context, lsp nb.LogicalSwitchPort, switchName string) *PortDiagnostic {
	diag := &PortDiagnostic{
		PortUUID:   lsp.UUID,
		PortName:   lsp.Name,
		PortType:   lsp.Type,
		SwitchName: switchName,
		Overall:    SeverityHealthy,
	}

	pb := d.findPortBinding(ctx, lsp.Name)

	// 1. Binding status
	diag.addCheck(d.checkBinding(lsp, pb))

	// 2. Port state
	diag.addCheck(d.checkPortState(lsp))

	// 3. Chassis health
	if pb != nil {
		diag.addCheck(d.checkChassisHealth(ctx, lsp, pb))
	}

	// 4. Type consistency
	if pb != nil {
		diag.addCheck(d.checkTypeConsistency(lsp, pb))
	}

	// 5. Address config
	diag.addCheck(d.checkAddresses(lsp))

	// 6. Router peer
	if lsp.Type == "router" {
		diag.addCheck(d.checkRouterPeer(ctx, lsp))
	}

	// 7. Patch peer
	if lsp.Type == "patch" {
		diag.addCheck(d.checkPatchPeer(ctx, lsp))
	}

	// 8. Stale binding chassis
	if pb != nil && pb.Chassis != nil && *pb.Chassis != "" {
		diag.addCheck(d.checkStaleChassis(ctx, pb))
	}

	return diag
}

func (diag *PortDiagnostic) addCheck(check DiagnosticCheck) {
	diag.Checks = append(diag.Checks, check)
	if check.Status == SeverityError && diag.Overall != SeverityError {
		diag.Overall = SeverityError
	} else if check.Status == SeverityWarning && diag.Overall == SeverityHealthy {
		diag.Overall = SeverityWarning
	}
}

func (d *PortDiagnoser) checkBinding(lsp nb.LogicalSwitchPort, pb *sb.PortBinding) DiagnosticCheck {
	if pb != nil {
		return DiagnosticCheck{
			Name:    "binding_status",
			Status:  SeverityHealthy,
			Message: "Port binding exists",
		}
	}
	// VIF ports (type "") without binding is an error; other types may not need one
	if lsp.Type == "" {
		return DiagnosticCheck{
			Name:    "binding_status",
			Status:  SeverityError,
			Message: "No port binding found for VIF port",
		}
	}
	return DiagnosticCheck{
		Name:    "binding_status",
		Status:  SeverityWarning,
		Message: fmt.Sprintf("No port binding found (type=%s)", lsp.Type),
	}
}

func (d *PortDiagnoser) checkPortState(lsp nb.LogicalSwitchPort) DiagnosticCheck {
	if lsp.Enabled != nil && !*lsp.Enabled {
		return DiagnosticCheck{
			Name:    "port_state",
			Status:  SeverityWarning,
			Message: "Port is administratively disabled",
		}
	}
	if lsp.Up != nil && !*lsp.Up {
		return DiagnosticCheck{
			Name:    "port_state",
			Status:  SeverityWarning,
			Message: "Port is down",
		}
	}
	return DiagnosticCheck{
		Name:    "port_state",
		Status:  SeverityHealthy,
		Message: "Port is up and enabled",
	}
}

func (d *PortDiagnoser) checkChassisHealth(ctx context.Context, lsp nb.LogicalSwitchPort, pb *sb.PortBinding) DiagnosticCheck {
	if pb.Chassis == nil || *pb.Chassis == "" {
		// VIF ports should be bound to a chassis
		if lsp.Type == "" {
			return DiagnosticCheck{
				Name:    "chassis_health",
				Status:  SeverityError,
				Message: "VIF port not bound to any chassis",
			}
		}
		return DiagnosticCheck{
			Name:    "chassis_health",
			Status:  SeverityHealthy,
			Message: fmt.Sprintf("No chassis binding expected for type=%s", lsp.Type),
		}
	}

	ch := &sb.Chassis{UUID: *pb.Chassis}
	if err := d.SB.Get(ctx, ch); err != nil {
		return DiagnosticCheck{
			Name:    "chassis_health",
			Status:  SeverityError,
			Message: "Bound chassis not found in SB database",
		}
	}

	return DiagnosticCheck{
		Name:    "chassis_health",
		Status:  SeverityHealthy,
		Message: fmt.Sprintf("Bound to chassis %s (%s)", ch.Name, ch.Hostname),
	}
}

func (d *PortDiagnoser) checkTypeConsistency(lsp nb.LogicalSwitchPort, pb *sb.PortBinding) DiagnosticCheck {
	if lsp.Type == pb.Type {
		return DiagnosticCheck{
			Name:    "type_consistency",
			Status:  SeverityHealthy,
			Message: fmt.Sprintf("LSP and PortBinding types match (%q)", lsp.Type),
		}
	}
	return DiagnosticCheck{
		Name:    "type_consistency",
		Status:  SeverityWarning,
		Message: fmt.Sprintf("LSP type %q != PortBinding type %q", lsp.Type, pb.Type),
	}
}

func (d *PortDiagnoser) checkAddresses(lsp nb.LogicalSwitchPort) DiagnosticCheck {
	if lsp.Type != "" {
		return DiagnosticCheck{
			Name:    "address_config",
			Status:  SeverityHealthy,
			Message: fmt.Sprintf("Address check skipped for type=%s", lsp.Type),
		}
	}
	if len(lsp.Addresses) == 0 {
		return DiagnosticCheck{
			Name:    "address_config",
			Status:  SeverityWarning,
			Message: "VIF port has no addresses configured",
		}
	}
	return DiagnosticCheck{
		Name:    "address_config",
		Status:  SeverityHealthy,
		Message: fmt.Sprintf("%d address(es) configured", len(lsp.Addresses)),
	}
}

func (d *PortDiagnoser) checkRouterPeer(ctx context.Context, lsp nb.LogicalSwitchPort) DiagnosticCheck {
	routerPort, ok := lsp.Options["router-port"]
	if !ok || routerPort == "" {
		return DiagnosticCheck{
			Name:    "router_peer",
			Status:  SeverityError,
			Message: "Router port has no router-port option set",
		}
	}

	var lrps []nb.LogicalRouterPort
	err := d.NB.WhereCache(func(lrp *nb.LogicalRouterPort) bool {
		return lrp.Name == routerPort
	}).List(ctx, &lrps)

	if err != nil || len(lrps) == 0 {
		return DiagnosticCheck{
			Name:    "router_peer",
			Status:  SeverityError,
			Message: fmt.Sprintf("Referenced router port %q not found", routerPort),
		}
	}
	return DiagnosticCheck{
		Name:    "router_peer",
		Status:  SeverityHealthy,
		Message: fmt.Sprintf("Router peer %q exists", routerPort),
	}
}

func (d *PortDiagnoser) checkPatchPeer(ctx context.Context, lsp nb.LogicalSwitchPort) DiagnosticCheck {
	peer, ok := lsp.Options["peer"]
	if !ok || peer == "" {
		return DiagnosticCheck{
			Name:    "patch_peer",
			Status:  SeverityError,
			Message: "Patch port has no peer option set",
		}
	}

	var lsps []nb.LogicalSwitchPort
	err := d.NB.WhereCache(func(p *nb.LogicalSwitchPort) bool {
		return p.Name == peer
	}).List(ctx, &lsps)

	if err != nil || len(lsps) == 0 {
		return DiagnosticCheck{
			Name:    "patch_peer",
			Status:  SeverityError,
			Message: fmt.Sprintf("Referenced patch peer %q not found", peer),
		}
	}
	return DiagnosticCheck{
		Name:    "patch_peer",
		Status:  SeverityHealthy,
		Message: fmt.Sprintf("Patch peer %q exists", peer),
	}
}

func (d *PortDiagnoser) checkStaleChassis(ctx context.Context, pb *sb.PortBinding) DiagnosticCheck {
	ch := &sb.Chassis{UUID: *pb.Chassis}
	if err := d.SB.Get(ctx, ch); err != nil {
		return DiagnosticCheck{
			Name:    "stale_chassis",
			Status:  SeverityError,
			Message: fmt.Sprintf("Chassis UUID %s no longer exists", *pb.Chassis),
		}
	}
	return DiagnosticCheck{
		Name:    "stale_chassis",
		Status:  SeverityHealthy,
		Message: "Chassis reference is valid",
	}
}

func (d *PortDiagnoser) findPortBinding(ctx context.Context, logicalPort string) *sb.PortBinding {
	var pbs []sb.PortBinding
	if err := d.SB.WhereCache(func(pb *sb.PortBinding) bool {
		return pb.LogicalPort == logicalPort
	}).List(ctx, &pbs); err != nil {
		return nil
	}
	if len(pbs) == 0 {
		return nil
	}
	return &pbs[0]
}

func (d *PortDiagnoser) findParentSwitchName(ctx context.Context, lspUUID string) string {
	var switches []nb.LogicalSwitch
	if err := d.NB.WhereCache(func(ls *nb.LogicalSwitch) bool {
		for _, p := range ls.Ports {
			if p == lspUUID {
				return true
			}
		}
		return false
	}).List(ctx, &switches); err != nil {
		return ""
	}
	if len(switches) > 0 {
		return switches[0].Name
	}
	return ""
}

func (d *PortDiagnoser) buildSwitchNameMap(ctx context.Context) map[string]string {
	result := make(map[string]string)
	var switches []nb.LogicalSwitch
	if err := d.NB.List(ctx, &switches); err != nil {
		return result
	}
	for _, ls := range switches {
		for _, portUUID := range ls.Ports {
			result[portUUID] = ls.Name
		}
	}
	return result
}
