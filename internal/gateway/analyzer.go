// Package gateway analyzes OVN distributed gateway ports (the chassisredirect
// "cr-lrp" ports) to detect when the chassis actually serving north/south
// traffic diverges from the chassis the HA election should have chosen.
//
// The core signal is a comparison of two truths the OVN databases already hold:
//
//   - desired: the highest-priority, still-alive member of the gateway's
//     HA_Chassis_Group (or Gateway_Chassis list) — what ovn-northd/ovn-controller
//     should elect as the active gateway.
//   - actual:  Port_Binding.chassis of the chassisredirect port — the chassis
//     that currently owns the centralized SNAT/DNAT and answers ARP for the
//     external addresses.
//
// When desired != actual the failover is stuck (failback never happened); when
// the same external IP is served from more than one chassis there is a split /
// duplicate-announce condition. Both are read-only and computed entirely from
// the in-memory libovsdb cache.
package gateway

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// Severity mirrors debug.DiagnosticSeverity and the frontend severity vocabulary
// so the existing diagnostic rendering can be reused verbatim.
const (
	SeverityHealthy = "healthy"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// Status values are machine-readable and more specific than Severity.
const (
	StatusOK            = "ok"             // active chassis matches the elected master
	StatusUnmanaged     = "unmanaged"      // no HA group; gateway pinned to a single chassis
	StatusDegraded      = "degraded"       // correct owner, but it is stale (failover likely imminent)
	StatusFailoverStuck = "failover-stuck" // active != desired: failback pending / election stuck
	StatusNoOwner       = "no-owner"       // chassisredirect port has no chassis (traffic blackholed)
	StatusNoCandidate   = "no-viable-candidate"
)

const defaultStaleThreshold = 2

// Analyzer cross-references NB intent with SB realization.
type Analyzer struct {
	NB client.Client
	SB client.Client
	// StaleThreshold is the nb_cfg lag beyond which a chassis is treated as not
	// alive for gateway election. Zero falls back to defaultStaleThreshold.
	StaleThreshold int
}

// Check is a single diagnostic result for a gateway.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Member is one candidate chassis in a gateway's HA group / gateway-chassis list.
type Member struct {
	ChassisUUID    string `json:"chassis_uuid,omitempty"`
	Name           string `json:"name"`
	Hostname       string `json:"hostname,omitempty"`
	Priority       int    `json:"priority"`
	Present        bool   `json:"present"`
	Stale          bool   `json:"stale"`
	GatewayCapable bool   `json:"gateway_capable"`
	Alive          bool   `json:"alive"`
	Active         bool   `json:"active"`  // == the actual owner
	Desired        bool   `json:"desired"` // == the elected master
}

// Gateway is the analysis result for a single chassisredirect port.
type Gateway struct {
	CRPort           string   `json:"cr_port"`
	PortBindingUUID  string   `json:"port_binding_uuid"`
	LRPName          string   `json:"lrp_name,omitempty"`
	RouterUUID       string   `json:"router_uuid,omitempty"`
	RouterName       string   `json:"router_name,omitempty"`
	HAGroupUUID      string   `json:"ha_group_uuid,omitempty"`
	HAGroupName      string   `json:"ha_group_name,omitempty"`
	ExternalNetworks []string `json:"external_networks,omitempty"`
	ServedIPs        []string `json:"served_ips,omitempty"`
	DesiredChassis   string   `json:"desired_chassis,omitempty"`
	ActualChassis    string   `json:"actual_chassis,omitempty"`
	Members          []Member `json:"members"`
	Status           string   `json:"status"`
	Overall          string   `json:"overall"`
	Checks           []Check  `json:"checks"`
}

// Conflict reports an external IP served from more than one chassis at once.
type Conflict struct {
	ExternalIP string   `json:"external_ip"`
	Chassis    []string `json:"chassis"`
	CRPorts    []string `json:"cr_ports"`
}

// Report aggregates gateway diagnostics across the deployment.
type Report struct {
	Total     int        `json:"total"`
	Healthy   int        `json:"healthy"`
	Warning   int        `json:"warning"`
	Error     int        `json:"error"`
	Gateways  []Gateway  `json:"gateways"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
}

// index holds the lookups built once per Analyze call.
type index struct {
	chassis      map[string]sb.Chassis
	haChassis    map[string]sb.HAChassis
	haGroups     map[string]sb.HAChassisGroup
	gwChassis    map[string]sb.GatewayChassis
	lrpByName    map[string]nb.LogicalRouterPort
	routerByLRP  map[string]nb.LogicalRouter
	natsByRouter map[string][]nb.NAT
	nbCfgRef     int
	nbCfgOK      bool
}

// Analyze scans all chassisredirect port bindings and returns the health report.
func (a *Analyzer) Analyze(ctx context.Context) (*Report, error) {
	var bindings []sb.PortBinding
	if err := a.SB.List(ctx, &bindings); err != nil {
		return nil, fmt.Errorf("listing port bindings: %w", err)
	}

	idx, err := a.buildIndex(ctx)
	if err != nil {
		return nil, err
	}

	report := &Report{}
	for _, pb := range bindings {
		if pb.Type != "chassisredirect" {
			continue
		}
		gw := a.analyzeGateway(pb, idx)
		switch gw.Overall {
		case SeverityHealthy:
			report.Healthy++
		case SeverityWarning:
			report.Warning++
		case SeverityError:
			report.Error++
		}
		report.Gateways = append(report.Gateways, gw)
	}
	report.Total = len(report.Gateways)
	report.Conflicts = detectConflicts(report.Gateways)

	// Errors first, then warnings, then healthy; stable within a tier so the
	// order stays deterministic for a given cache snapshot.
	sort.SliceStable(report.Gateways, func(i, j int) bool {
		return severityOrder(report.Gateways[i].Overall) < severityOrder(report.Gateways[j].Overall)
	})
	return report, nil
}

func (a *Analyzer) analyzeGateway(pb sb.PortBinding, idx *index) Gateway {
	gw := Gateway{
		CRPort:          pb.LogicalPort,
		PortBindingUUID: pb.UUID,
		Overall:         SeverityHealthy,
	}

	// Resolve the NB router port behind the cr-lrp ("cr-" prefix), and from it
	// the owning router, its external networks, and the IPs it serves.
	gw.LRPName = strings.TrimPrefix(pb.LogicalPort, "cr-")
	served := newStringSet()
	if lrp, ok := idx.lrpByName[gw.LRPName]; ok {
		for _, n := range lrp.Networks {
			gw.ExternalNetworks = append(gw.ExternalNetworks, n)
			served.add(ipOnly(n))
		}
		if lr, ok := idx.routerByLRP[lrp.UUID]; ok {
			gw.RouterUUID = lr.UUID
			gw.RouterName = lr.Name
			for _, nat := range idx.natsByRouter[lr.UUID] {
				served.add(nat.ExternalIP)
			}
		}
	}
	gw.ServedIPs = served.sorted()

	members := a.buildMembers(pb, idx, &gw)
	gw.Members = members

	actualUUID := derefStr(pb.Chassis)
	desiredUUID := ""
	for i := range members {
		if members[i].ChassisUUID == actualUUID && actualUUID != "" {
			members[i].Active = true
		}
		if members[i].Desired {
			desiredUUID = members[i].ChassisUUID
		}
	}
	actualState := a.chassisState(actualUUID, idx)
	gw.ActualChassis = actualState.displayName(actualUUID)

	a.appendChecks(&gw, members, actualUUID, desiredUUID, actualState)
	if gw.Status == "" {
		gw.Status = StatusOK
	}
	return gw
}

// buildMembers resolves the HA group (preferred) or gateway-chassis list into
// candidate members, marks the highest-priority alive one as desired, and fills
// in the HA group metadata + DesiredChassis on gw.
func (a *Analyzer) buildMembers(pb sb.PortBinding, idx *index, gw *Gateway) []Member {
	type cand struct {
		chassisUUID string
		priority    int
	}
	var cands []cand

	switch {
	case pb.HaChassisGroup != nil:
		if grp, ok := idx.haGroups[*pb.HaChassisGroup]; ok {
			gw.HAGroupUUID = grp.UUID
			gw.HAGroupName = grp.Name
			for _, hacUUID := range grp.HaChassis {
				if hac, ok := idx.haChassis[hacUUID]; ok {
					cands = append(cands, cand{derefStr(hac.Chassis), hac.Priority})
				}
			}
		}
	case len(pb.GatewayChassis) > 0:
		for _, gcUUID := range pb.GatewayChassis {
			if gc, ok := idx.gwChassis[gcUUID]; ok {
				cands = append(cands, cand{derefStr(gc.Chassis), gc.Priority})
			}
		}
	}

	members := make([]Member, 0, len(cands))
	for _, c := range cands {
		st := a.chassisState(c.chassisUUID, idx)
		members = append(members, Member{
			ChassisUUID:    c.chassisUUID,
			Name:           st.displayName(c.chassisUUID),
			Hostname:       st.hostname,
			Priority:       c.priority,
			Present:        st.present,
			Stale:          st.stale,
			GatewayCapable: st.capable,
			Alive:          st.present && !st.stale,
		})
	}

	// Highest priority wins; alive members only. ovn-northd uses the same rule.
	best := -1
	for i := range members {
		if !members[i].Alive {
			continue
		}
		if best == -1 || members[i].Priority > members[best].Priority {
			best = i
		}
	}
	if best != -1 {
		members[best].Desired = true
		gw.DesiredChassis = members[best].Name
	}
	return members
}

func (a *Analyzer) appendChecks(gw *Gateway, members []Member, actualUUID, desiredUUID string, actual chState) {
	hasMembers := len(members) > 0

	// 1. Binding owner.
	if actualUUID == "" {
		gw.Status = StatusNoOwner
		gw.addCheck("binding_owner", SeverityError, "chassisredirect port has no active chassis — north/south traffic is blackholed")
	} else {
		gw.addCheck("binding_owner", SeverityHealthy, fmt.Sprintf("active on chassis %s", gw.ActualChassis))
	}

	// 2. Election: does the active chassis match the elected master?
	switch {
	case !hasMembers:
		if gw.Status == "" && actualUUID != "" {
			gw.Status = StatusUnmanaged
		}
		gw.addCheck("election", SeverityHealthy, "no HA chassis group — gateway is pinned to a single chassis")
	case desiredUUID == "":
		if gw.Status == "" {
			gw.Status = StatusNoCandidate
		}
		gw.addCheck("election", SeverityError, "no alive HA member is available to host the gateway")
	case actualUUID == "":
		gw.addCheck("election", SeverityError, fmt.Sprintf("expected active chassis %s, but the port is unbound", gw.DesiredChassis))
	case actualUUID != desiredUUID:
		gw.Status = StatusFailoverStuck
		gw.addCheck("election", SeverityError, fmt.Sprintf("active on %s, but %s has higher priority and is alive — failover stuck / failback pending", gw.ActualChassis, gw.DesiredChassis))
	default:
		gw.addCheck("election", SeverityHealthy, fmt.Sprintf("active chassis %s matches the highest-priority alive member", gw.ActualChassis))
	}

	// 3. Member liveness.
	if hasMembers {
		down := 0
		for _, m := range members {
			if !m.Alive {
				down++
			}
		}
		if down > 0 {
			gw.addCheck("members_liveness", SeverityWarning, fmt.Sprintf("%d of %d HA members are down or stale", down, len(members)))
		} else {
			gw.addCheck("members_liveness", SeverityHealthy, fmt.Sprintf("all %d HA members are alive", len(members)))
		}
	}

	// 4. Owner liveness — a correct but stale owner is a warning, not yet an error.
	if actualUUID != "" && actual.present && actual.stale {
		if gw.Status == StatusOK || gw.Status == "" || gw.Status == StatusUnmanaged {
			gw.Status = StatusDegraded
		}
		gw.addCheck("owner_liveness", SeverityWarning, "active chassis is stale (nb_cfg lagging) — a failover may be imminent")
	}

	// 5. Gateway capability — owner must advertise enable-chassis-as-gw.
	if actualUUID != "" && actual.present && !actual.capable {
		gw.addCheck("gateway_capability", SeverityWarning, "active chassis does not advertise enable-chassis-as-gw")
	}
}

func (gw *Gateway) addCheck(name, status, msg string) {
	gw.Checks = append(gw.Checks, Check{Name: name, Status: status, Message: msg})
	switch status {
	case SeverityError:
		gw.Overall = SeverityError
	case SeverityWarning:
		if gw.Overall != SeverityError {
			gw.Overall = SeverityWarning
		}
	}
}

// chState is the resolved liveness/capability of a chassis UUID.
type chState struct {
	present  bool
	stale    bool
	capable  bool
	name     string
	hostname string
}

func (s chState) displayName(uuid string) string {
	if s.name != "" {
		return s.name
	}
	if uuid == "" {
		return ""
	}
	if len(uuid) > 8 {
		return uuid[:8]
	}
	return uuid
}

func (a *Analyzer) chassisState(uuid string, idx *index) chState {
	if uuid == "" {
		return chState{}
	}
	ch, ok := idx.chassis[uuid]
	if !ok {
		return chState{} // not present => not alive
	}
	st := chState{present: true, name: ch.Name, hostname: ch.Hostname, capable: gatewayCapable(ch)}
	if idx.nbCfgOK && a.staleThreshold() > 0 && idx.nbCfgRef-ch.NbCfg > a.staleThreshold() {
		st.stale = true
	}
	return st
}

func (a *Analyzer) staleThreshold() int {
	if a.StaleThreshold <= 0 {
		return defaultStaleThreshold
	}
	return a.StaleThreshold
}

func (a *Analyzer) buildIndex(ctx context.Context) (*index, error) {
	idx := &index{
		chassis:      map[string]sb.Chassis{},
		haChassis:    map[string]sb.HAChassis{},
		haGroups:     map[string]sb.HAChassisGroup{},
		gwChassis:    map[string]sb.GatewayChassis{},
		lrpByName:    map[string]nb.LogicalRouterPort{},
		routerByLRP:  map[string]nb.LogicalRouter{},
		natsByRouter: map[string][]nb.NAT{},
	}

	var chassisList []sb.Chassis
	if err := a.SB.List(ctx, &chassisList); err != nil {
		return nil, fmt.Errorf("listing chassis: %w", err)
	}
	for _, c := range chassisList {
		idx.chassis[c.UUID] = c
	}

	var haChassisList []sb.HAChassis
	if err := a.SB.List(ctx, &haChassisList); err != nil {
		return nil, fmt.Errorf("listing ha_chassis: %w", err)
	}
	for _, h := range haChassisList {
		idx.haChassis[h.UUID] = h
	}

	var haGroupList []sb.HAChassisGroup
	if err := a.SB.List(ctx, &haGroupList); err != nil {
		return nil, fmt.Errorf("listing ha_chassis_group: %w", err)
	}
	for _, g := range haGroupList {
		idx.haGroups[g.UUID] = g
	}

	var gwChassisList []sb.GatewayChassis
	if err := a.SB.List(ctx, &gwChassisList); err != nil {
		return nil, fmt.Errorf("listing gateway_chassis: %w", err)
	}
	for _, g := range gwChassisList {
		idx.gwChassis[g.UUID] = g
	}

	var lrps []nb.LogicalRouterPort
	if err := a.NB.List(ctx, &lrps); err != nil {
		return nil, fmt.Errorf("listing logical router ports: %w", err)
	}
	for _, lrp := range lrps {
		idx.lrpByName[lrp.Name] = lrp
	}

	var natsByUUID = map[string]nb.NAT{}
	var nats []nb.NAT
	if err := a.NB.List(ctx, &nats); err != nil {
		return nil, fmt.Errorf("listing nats: %w", err)
	}
	for _, n := range nats {
		natsByUUID[n.UUID] = n
	}

	var routers []nb.LogicalRouter
	if err := a.NB.List(ctx, &routers); err != nil {
		return nil, fmt.Errorf("listing logical routers: %w", err)
	}
	for _, lr := range routers {
		for _, p := range lr.Ports {
			idx.routerByLRP[p] = lr
		}
		for _, natUUID := range lr.Nat {
			if n, ok := natsByUUID[natUUID]; ok {
				idx.natsByRouter[lr.UUID] = append(idx.natsByRouter[lr.UUID], n)
			}
		}
	}

	var globals []nb.NBGlobal
	if err := a.NB.List(ctx, &globals); err == nil && len(globals) > 0 {
		idx.nbCfgRef = globals[0].NbCfg
		idx.nbCfgOK = true
	}

	return idx, nil
}

// detectConflicts flags any external IP that is served from more than one
// distinct active chassis at the same time (overlapping FIP / split announce).
func detectConflicts(gws []Gateway) []Conflict {
	type agg struct {
		chassis *stringSet
		ports   *stringSet
	}
	byIP := map[string]*agg{}
	for _, gw := range gws {
		if gw.ActualChassis == "" {
			continue
		}
		for _, ip := range gw.ServedIPs {
			a := byIP[ip]
			if a == nil {
				a = &agg{chassis: newStringSet(), ports: newStringSet()}
				byIP[ip] = a
			}
			a.chassis.add(gw.ActualChassis)
			a.ports.add(gw.CRPort)
		}
	}

	var out []Conflict
	for ip, a := range byIP {
		if a.chassis.len() > 1 {
			out = append(out, Conflict{ExternalIP: ip, Chassis: a.chassis.sorted(), CRPorts: a.ports.sorted()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExternalIP < out[j].ExternalIP })
	return out
}

func gatewayCapable(ch sb.Chassis) bool {
	opts := ch.OtherConfig["ovn-cms-options"]
	if opts == "" {
		opts = ch.ExternalIDs["ovn-cms-options"]
	}
	if opts == "" {
		return true // unknown — don't raise a false positive
	}
	for _, o := range strings.Split(opts, ",") {
		if strings.TrimSpace(o) == "enable-chassis-as-gw" {
			return true
		}
	}
	return false
}

func severityOrder(s string) int {
	switch s {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ipOnly strips a CIDR mask, returning just the address part.
func ipOnly(cidr string) string {
	if i := strings.IndexByte(cidr, '/'); i != -1 {
		return cidr[:i]
	}
	return cidr
}

type stringSet struct{ m map[string]struct{} }

func newStringSet() *stringSet { return &stringSet{m: map[string]struct{}{}} }

func (s *stringSet) add(v string) {
	if v == "" {
		return
	}
	s.m[v] = struct{}{}
}

func (s *stringSet) len() int { return len(s.m) }

func (s *stringSet) sorted() []string {
	if len(s.m) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
