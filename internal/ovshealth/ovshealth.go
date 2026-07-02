// Package ovshealth aggregates fleet-wide Open_vSwitch health from the
// per-chassis libovsdb caches. Given the live bridges, ports and interfaces
// read from each chassis's monitored cache, it sums the totals across the fleet
// and counts interfaces that are down (link_state=down), erroring (rx/tx error
// counters that INCREASED since the previous aggregation) or dropping (rx/tx
// drop counters that increased), producing both fleet-wide totals and a
// per-chassis breakdown.
//
// Health is delta-based: Interface.statistics counters are cumulative since
// boot, so a single lifetime error or drop must not mark an interface erroring
// forever. A Tracker remembers the previous per-interface counters and flags an
// interface only when the counters advance between reads. Errors and drops are
// reported separately.
//
// Aggregation opens no OVSDB connections — it consumes caches the caller has
// already read — and handles partial outages: an unreachable chassis is
// excluded from every total (never counted as healthy) while still appearing in
// the per-chassis list with connected=false and zero counts.
package ovshealth

import (
	"sync"

	"github.com/b42labs/northwatch/internal/ovsdb/vs"
)

// errorKeys and dropKeys are the Interface.statistics counters that mark an
// interface as erroring or dropping respectively when they increase.
var (
	errorKeys = []string{"rx_errors", "tx_errors"}
	dropKeys  = []string{"rx_dropped", "tx_dropped"}
)

// ChassisCache is the live per-chassis OVS state the aggregation consumes: the
// chassis system-id, whether Northwatch is currently connected to it, and the
// bridges/ports/interfaces read from its monitored cache. The slices of an
// unreachable chassis (Connected=false) are ignored.
type ChassisCache struct {
	SystemID   string
	Connected  bool
	Bridges    []vs.Bridge
	Ports      []vs.Port
	Interfaces []vs.Interface
}

// ChassisHealth is the per-chassis health breakdown. For an unreachable chassis
// (Connected=false) every count is zero: its cache is not read, so it
// contributes nothing to the fleet totals.
type ChassisHealth struct {
	SystemID        string `json:"system_id"`
	Connected       bool   `json:"connected"`
	Bridges         int    `json:"bridges"`
	Ports           int    `json:"ports"`
	Interfaces      int    `json:"interfaces"`
	DownInterfaces  int    `json:"down_interfaces"`
	ErrorInterfaces int    `json:"error_interfaces"`
	DropInterfaces  int    `json:"drop_interfaces"`
}

// FleetHealth is the aggregated fleet-wide OVS health: connection counts, the
// summed bridge/port/interface totals across connected chassis, the down and
// erroring interface counts, and the per-chassis breakdown in input order.
// Unreachable chassis are counted in Chassis and Unreachable but excluded from
// every other total.
type FleetHealth struct {
	Chassis         int             `json:"chassis"`
	Connected       int             `json:"connected"`
	Unreachable     int             `json:"unreachable"`
	Bridges         int             `json:"bridges"`
	Ports           int             `json:"ports"`
	Interfaces      int             `json:"interfaces"`
	DownInterfaces  int             `json:"down_interfaces"`
	ErrorInterfaces int             `json:"error_interfaces"`
	DropInterfaces  int             `json:"drop_interfaces"`
	Members         []ChassisHealth `json:"members"`
}

// Tracker aggregates fleet OVS health across successive reads. It remembers the
// previous per-interface error/drop counters so an interface is flagged as
// erroring or dropping only when its cumulative counters INCREASE between
// aggregations — a single lifetime error no longer marks it erroring forever,
// and a counter reset (or first observation) establishes a baseline without
// flagging. It is safe for concurrent use.
type Tracker struct {
	mu   sync.Mutex
	prev map[string]map[string]int // interface key -> counter snapshot
}

// NewTracker returns a Tracker with an empty baseline.
func NewTracker() *Tracker {
	return &Tracker{prev: make(map[string]map[string]int)}
}

// Aggregate sums the per-chassis caches into a FleetHealth. It preserves input
// order in Members and counts bridges, ports, interfaces, and the down/erroring/
// dropping interfaces only for connected chassis, so an unreachable chassis is
// excluded from every total rather than counted as healthy. Erroring and
// dropping are computed from counter deltas against the previous call; state for
// chassis/interfaces absent from this call is pruned.
func (t *Tracker) Aggregate(caches []ChassisCache) FleetHealth {
	t.mu.Lock()
	defer t.mu.Unlock()

	next := make(map[string]map[string]int)
	fleet := FleetHealth{
		Chassis: len(caches),
		Members: make([]ChassisHealth, 0, len(caches)),
	}
	for _, cc := range caches {
		ch := ChassisHealth{SystemID: cc.SystemID, Connected: cc.Connected}
		if cc.Connected {
			fleet.Connected++
			ch.Bridges = len(cc.Bridges)
			ch.Ports = len(cc.Ports)
			ch.Interfaces = len(cc.Interfaces)
			for _, iface := range cc.Interfaces {
				if interfaceDown(iface) {
					ch.DownInterfaces++
				}
				key := cc.SystemID + "\x00" + iface.UUID
				cur := counterSnapshot(iface)
				next[key] = cur
				prev, seen := t.prev[key]
				if !seen {
					continue // first observation: baseline only
				}
				if anyIncreased(prev, cur, errorKeys) {
					ch.ErrorInterfaces++
				}
				if anyIncreased(prev, cur, dropKeys) {
					ch.DropInterfaces++
				}
			}
			fleet.Bridges += ch.Bridges
			fleet.Ports += ch.Ports
			fleet.Interfaces += ch.Interfaces
			fleet.DownInterfaces += ch.DownInterfaces
			fleet.ErrorInterfaces += ch.ErrorInterfaces
			fleet.DropInterfaces += ch.DropInterfaces
		}
		fleet.Members = append(fleet.Members, ch)
	}
	t.prev = next // prune vanished chassis/interfaces
	fleet.Unreachable = fleet.Chassis - fleet.Connected
	return fleet
}

// counterSnapshot captures the error and drop counters of an interface.
func counterSnapshot(iface vs.Interface) map[string]int {
	m := make(map[string]int, len(errorKeys)+len(dropKeys))
	for _, k := range errorKeys {
		m[k] = iface.Statistics[k]
	}
	for _, k := range dropKeys {
		m[k] = iface.Statistics[k]
	}
	return m
}

// anyIncreased reports whether any of the named counters is larger in cur than
// in prev.
func anyIncreased(prev, cur map[string]int, keys []string) bool {
	for _, k := range keys {
		if cur[k] > prev[k] {
			return true
		}
	}
	return false
}

// interfaceDown reports whether an interface's link_state is down. A nil
// link_state (absent from the OVSDB row) is not down.
func interfaceDown(iface vs.Interface) bool {
	return iface.LinkState != nil && *iface.LinkState == vs.InterfaceLinkStateDown
}
