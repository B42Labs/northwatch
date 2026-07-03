// Package router analyzes the next-hop MAC resolution of OVN logical routers.
//
// An OVN logical router learns the MAC of a static route's next hop via ARP/ND
// and caches it in the Southbound MAC_Binding table (ip, mac, logical_port,
// timestamp). If the next-hop device changes its MAC — e.g. a tap interface
// without a fixed MAC — and the router has no ARP-cache aging configured, the
// stale entry is never refreshed and traffic to that next hop blackholes. See
// https://weiti.org/ovn-logical-router-and-changed-next-hop-mac.html
//
// Northwatch is read-only and cannot ARP, so it cannot prove a cached MAC is
// wrong. What it can do — entirely from the NB/SB libovsdb caches — is detect
// the conditions that make this failure mode possible and surface the cached
// next-hop MACs correlated with the static routes that depend on them:
//
//   - no-aging:     a dynamic MAC_Binding exists but neither the router's
//     options nor NB_Global sets mac_binding_age_threshold, so the entry never
//     expires (the article's exact precondition).
//   - stale:        aging IS configured, yet an entry is older than the
//     threshold — ovn-northd should have aged it out (control-plane anomaly).
//   - mac-conflict: a Static_MAC_Binding pins a MAC that disagrees with the
//     learned dynamic MAC_Binding for the same next hop.
//   - unresolved:   the static route's next hop has neither a dynamic nor a
//     static MAC binding (informational — may simply be not-yet-learned).
package router

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/severity"
)

// Severity mirrors debug.DiagnosticSeverity and gateway.Severity so the
// existing frontend severity rendering (lib/status.ts) is reused verbatim.
const (
	SeverityHealthy = severity.Healthy
	SeverityWarning = severity.Warning
	SeverityError   = severity.Error
)

// Status values are machine-readable and more specific than Severity.
const (
	StatusOK         = "ok"           // dynamic MAC learned, aging configured, not stale
	StatusPinned     = "pinned"       // a Static_MAC_Binding fixes the next-hop MAC
	StatusNoAging    = "no-aging"     // learned MAC never expires (article's risk)
	StatusStale      = "stale"        // older than the configured aging threshold
	StatusConflict   = "mac-conflict" // static vs dynamic MAC disagree
	StatusUnresolved = "unresolved"   // next hop has no MAC binding at all
)

// thresholdOption is the OVN option that enables ARP-cache aging (seconds).
const thresholdOption = "mac_binding_age_threshold"

// Analyzer cross-references NB static routes with the SB MAC binding tables.
type Analyzer struct {
	NB client.Client
	SB client.Client
	// Now is injectable for deterministic tests; defaults to time.Now.
	Now func() time.Time
}

// Check is a single diagnostic result for a next hop.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NextHop is the analysis result for one static route's next hop.
type NextHop struct {
	RouterUUID     string  `json:"router_uuid"`
	RouterName     string  `json:"router_name"`
	RouteUUID      string  `json:"route_uuid"`
	IPPrefix       string  `json:"ip_prefix"`
	RouteTable     string  `json:"route_table,omitempty"`
	Nexthop        string  `json:"nexthop"`
	LRPName        string  `json:"lrp_name,omitempty"`
	CachedMAC      string  `json:"cached_mac,omitempty"`
	MACBindingUUID string  `json:"mac_binding_uuid,omitempty"`
	StaticMAC      string  `json:"static_mac,omitempty"`
	Override       bool    `json:"override_dynamic_mac,omitempty"`
	AgingEnabled   bool    `json:"aging_enabled"`
	AgeThreshold   int     `json:"age_threshold_seconds,omitempty"`
	AgeSeconds     int64   `json:"age_seconds,omitempty"`
	HasTimestamp   bool    `json:"has_timestamp"`
	Status         string  `json:"status"`
	Overall        string  `json:"overall"`
	Checks         []Check `json:"checks"`
}

// Report aggregates next-hop diagnostics across the deployment.
type Report struct {
	Total      int       `json:"total"`
	Healthy    int       `json:"healthy"`
	Warning    int       `json:"warning"`
	Error      int       `json:"error"`
	NoAging    int       `json:"no_aging"`
	Stale      int       `json:"stale"`
	Conflict   int       `json:"mac_conflict"`
	Unresolved int       `json:"unresolved"`
	NextHops   []NextHop `json:"next_hops"`
}

type index struct {
	lrpByUUID    map[string]nb.LogicalRouterPort
	routeByUUID  map[string]nb.LogicalRouterStaticRoute
	macByPortIP  map[string]sb.MACBinding         // "port|ip" -> binding
	statByPortIP map[string]sb.StaticMACBinding   // "port|ip" -> binding
	macByIP      map[string][]sb.MACBinding       // "ip" -> bindings (fallback)
	statByIP     map[string][]sb.StaticMACBinding // "ip" -> bindings (fallback)
	globalAge    int
}

// Analyze scans every logical router's static routes and returns the report.
func (a *Analyzer) Analyze(ctx context.Context) (*Report, error) {
	var routers []nb.LogicalRouter
	if err := a.NB.List(ctx, &routers); err != nil {
		return nil, fmt.Errorf("listing logical routers: %w", err)
	}

	idx, err := a.buildIndex(ctx)
	if err != nil {
		return nil, err
	}

	report := &Report{NextHops: []NextHop{}}
	for _, lr := range routers {
		routerLRPs := make([]nb.LogicalRouterPort, 0, len(lr.Ports))
		for _, p := range lr.Ports {
			if lrp, ok := idx.lrpByUUID[p]; ok {
				routerLRPs = append(routerLRPs, lrp)
			}
		}

		threshold := idx.globalAge
		if v, ok := parseThreshold(lr.Options[thresholdOption]); ok {
			threshold = v
		}

		for _, ruuid := range lr.StaticRoutes {
			route, ok := idx.routeByUUID[ruuid]
			if !ok {
				continue
			}
			nh := strings.TrimSpace(route.Nexthop)
			if nh == "" || nh == "discard" {
				continue // blackhole / directly-connected route, no next-hop MAC
			}
			report.NextHops = append(report.NextHops, a.analyzeNextHop(lr, route, routerLRPs, threshold, idx))
		}
	}

	for i := range report.NextHops {
		switch report.NextHops[i].Overall {
		case SeverityHealthy:
			report.Healthy++
		case SeverityWarning:
			report.Warning++
		case SeverityError:
			report.Error++
		}
		switch report.NextHops[i].Status {
		case StatusNoAging:
			report.NoAging++
		case StatusStale:
			report.Stale++
		case StatusConflict:
			report.Conflict++
		case StatusUnresolved:
			report.Unresolved++
		}
	}
	report.Total = len(report.NextHops)

	// Warnings first, then healthy; stable for a deterministic snapshot order.
	sort.SliceStable(report.NextHops, func(i, j int) bool {
		return severityOrder(report.NextHops[i].Overall) < severityOrder(report.NextHops[j].Overall)
	})
	return report, nil
}

func (a *Analyzer) analyzeNextHop(lr nb.LogicalRouter, route nb.LogicalRouterStaticRoute, routerLRPs []nb.LogicalRouterPort, threshold int, idx *index) NextHop {
	nh := strings.TrimSpace(route.Nexthop)
	hop := NextHop{
		RouterUUID:   lr.UUID,
		RouterName:   lr.Name,
		RouteUUID:    route.UUID,
		IPPrefix:     route.IPPrefix,
		Nexthop:      nh,
		AgingEnabled: threshold > 0,
		AgeThreshold: threshold,
		Overall:      SeverityHealthy,
		Status:       StatusOK,
	}
	if route.RouteTable != "" {
		hop.RouteTable = route.RouteTable
	}

	// Resolve the egress LRP: an explicit output_port wins, else the router LRP
	// whose connected subnet contains the next hop.
	egress := resolveEgressLRP(route, routerLRPs)
	if egress != nil {
		hop.LRPName = egress.Name
	}

	// Candidate logical_port names to match MAC bindings against.
	ports := candidatePorts(egress, routerLRPs)

	if mac, ok := lookupMAC(idx, ports, nh); ok {
		hop.CachedMAC = mac.MAC
		hop.MACBindingUUID = mac.UUID
		if hop.LRPName == "" {
			hop.LRPName = mac.LogicalPort
		}
		if mac.Timestamp > 0 {
			hop.HasTimestamp = true
			age := a.now().Sub(time.UnixMilli(int64(mac.Timestamp)))
			if age < 0 {
				age = 0
			}
			hop.AgeSeconds = int64(age.Seconds())
		}
	}
	if smac, ok := lookupStatic(idx, ports, nh); ok {
		hop.StaticMAC = smac.MAC
		hop.Override = smac.OverrideDynamicMAC
		if hop.LRPName == "" {
			hop.LRPName = smac.LogicalPort
		}
	}

	a.classify(&hop)
	return hop
}

// classify decides the status + checks from the gathered facts.
func (a *Analyzer) classify(hop *NextHop) {
	hasDynamic := hop.CachedMAC != ""
	hasStatic := hop.StaticMAC != ""

	switch {
	case hasStatic && hasDynamic && !strings.EqualFold(hop.StaticMAC, hop.CachedMAC) && !hop.Override:
		// Both present, they disagree, and the static one does not override.
		hop.set(StatusConflict, SeverityWarning, "static_vs_dynamic",
			fmt.Sprintf("Static_MAC_Binding pins %s but the learned MAC is %s (override_dynamic_mac is false)", hop.StaticMAC, hop.CachedMAC))

	case hasStatic:
		// A static binding pins the next-hop MAC — the article's recommended fix.
		msg := fmt.Sprintf("next-hop MAC pinned to %s via Static_MAC_Binding", hop.StaticMAC)
		if hasDynamic && hop.Override && !strings.EqualFold(hop.StaticMAC, hop.CachedMAC) {
			msg += fmt.Sprintf(" (overrides learned %s)", hop.CachedMAC)
		}
		hop.set(StatusPinned, SeverityHealthy, "static_binding", msg)

	case !hasDynamic:
		// No binding at all — informational, not an error.
		hop.set(StatusUnresolved, SeverityHealthy, "resolution",
			"next hop has no MAC_Binding yet (not learned, or no traffic has triggered ARP/ND)")

	case hop.AgingEnabled && hop.HasTimestamp && hop.AgeSeconds > int64(hop.AgeThreshold):
		hop.set(StatusStale, SeverityWarning, "aging",
			fmt.Sprintf("learned MAC %s is %ds old, beyond mac_binding_age_threshold=%ds — ovn-northd should have aged it out", hop.CachedMAC, hop.AgeSeconds, hop.AgeThreshold))

	case !hop.AgingEnabled:
		hop.set(StatusNoAging, SeverityWarning, "aging",
			fmt.Sprintf("learned MAC %s never expires — mac_binding_age_threshold is unset, so a changed next-hop MAC will not refresh", hop.CachedMAC))

	default:
		hop.set(StatusOK, SeverityHealthy, "aging",
			fmt.Sprintf("learned MAC %s, aging enabled (threshold=%ds)", hop.CachedMAC, hop.AgeThreshold))
	}
}

func (hop *NextHop) set(status, severity, checkName, msg string) {
	hop.Status = status
	hop.Checks = append(hop.Checks, Check{Name: checkName, Status: severity, Message: msg})
	switch severity {
	case SeverityError:
		hop.Overall = SeverityError
	case SeverityWarning:
		if hop.Overall != SeverityError {
			hop.Overall = SeverityWarning
		}
	}
}

func (a *Analyzer) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// resolveEgressLRP returns the router port a next hop egresses through: the
// explicit output_port if set, otherwise the LRP whose subnet contains it.
func resolveEgressLRP(route nb.LogicalRouterStaticRoute, lrps []nb.LogicalRouterPort) *nb.LogicalRouterPort {
	if route.OutputPort != nil && *route.OutputPort != "" {
		for i := range lrps {
			if lrps[i].Name == *route.OutputPort {
				return &lrps[i]
			}
		}
	}
	nh := net.ParseIP(strings.TrimSpace(route.Nexthop))
	if nh == nil {
		return nil
	}
	for i := range lrps {
		for _, cidr := range lrps[i].Networks {
			if _, ipnet, err := net.ParseCIDR(cidr); err == nil && ipnet.Contains(nh) {
				return &lrps[i]
			}
		}
	}
	return nil
}

// candidatePorts is the set of logical_port names to match a binding against:
// just the egress port when known, else every router LRP as a fallback.
func candidatePorts(egress *nb.LogicalRouterPort, lrps []nb.LogicalRouterPort) []string {
	if egress != nil {
		return []string{egress.Name}
	}
	names := make([]string, 0, len(lrps))
	for _, lrp := range lrps {
		names = append(names, lrp.Name)
	}
	return names
}

func lookupMAC(idx *index, ports []string, ip string) (sb.MACBinding, bool) {
	cip := canonicalIP(ip)
	for _, p := range ports {
		if mac, ok := idx.macByPortIP[p+"|"+cip]; ok {
			return mac, true
		}
	}
	// Fallback: any binding for this IP (egress port could not be resolved).
	if len(ports) == 0 {
		if list := idx.macByIP[cip]; len(list) == 1 {
			return list[0], true
		}
	}
	return sb.MACBinding{}, false
}

func lookupStatic(idx *index, ports []string, ip string) (sb.StaticMACBinding, bool) {
	cip := canonicalIP(ip)
	for _, p := range ports {
		if smac, ok := idx.statByPortIP[p+"|"+cip]; ok {
			return smac, true
		}
	}
	if len(ports) == 0 {
		if list := idx.statByIP[cip]; len(list) == 1 {
			return list[0], true
		}
	}
	return sb.StaticMACBinding{}, false
}

func (a *Analyzer) buildIndex(ctx context.Context) (*index, error) {
	idx := &index{
		lrpByUUID:    map[string]nb.LogicalRouterPort{},
		routeByUUID:  map[string]nb.LogicalRouterStaticRoute{},
		macByPortIP:  map[string]sb.MACBinding{},
		statByPortIP: map[string]sb.StaticMACBinding{},
		macByIP:      map[string][]sb.MACBinding{},
		statByIP:     map[string][]sb.StaticMACBinding{},
	}

	var lrps []nb.LogicalRouterPort
	if err := a.NB.List(ctx, &lrps); err != nil {
		return nil, fmt.Errorf("listing logical router ports: %w", err)
	}
	for _, lrp := range lrps {
		idx.lrpByUUID[lrp.UUID] = lrp
	}

	var routes []nb.LogicalRouterStaticRoute
	if err := a.NB.List(ctx, &routes); err != nil {
		return nil, fmt.Errorf("listing static routes: %w", err)
	}
	for _, r := range routes {
		idx.routeByUUID[r.UUID] = r
	}

	var macs []sb.MACBinding
	if err := a.SB.List(ctx, &macs); err != nil {
		return nil, fmt.Errorf("listing mac bindings: %w", err)
	}
	for _, m := range macs {
		cip := canonicalIP(m.IP)
		idx.macByPortIP[m.LogicalPort+"|"+cip] = m
		idx.macByIP[cip] = append(idx.macByIP[cip], m)
	}

	var stats []sb.StaticMACBinding
	if err := a.SB.List(ctx, &stats); err != nil {
		return nil, fmt.Errorf("listing static mac bindings: %w", err)
	}
	for _, s := range stats {
		cip := canonicalIP(s.IP)
		idx.statByPortIP[s.LogicalPort+"|"+cip] = s
		idx.statByIP[cip] = append(idx.statByIP[cip], s)
	}

	var globals []nb.NBGlobal
	if err := a.NB.List(ctx, &globals); err == nil && len(globals) > 0 {
		if v, ok := parseThreshold(globals[0].Options[thresholdOption]); ok {
			idx.globalAge = v
		}
	}

	return idx, nil
}

func parseThreshold(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

// canonicalIP normalizes an address for map-key comparison; non-IP strings pass
// through unchanged.
func canonicalIP(s string) string {
	s = strings.TrimSpace(s)
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return s
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
