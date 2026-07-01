// Package ovshealth aggregates fleet-wide Open_vSwitch health from the
// per-chassis libovsdb caches. Given the live bridges, ports and interfaces
// read from each chassis's monitored cache, it sums the totals across the fleet
// and counts interfaces that are down (link_state=down) or erroring (non-zero
// rx/tx errors or drops), producing both fleet-wide totals and a per-chassis
// breakdown.
//
// Aggregation opens no OVSDB connections — it consumes caches the caller has
// already read — and handles partial outages: an unreachable chassis is
// excluded from every total (never counted as healthy) while still appearing in
// the per-chassis list with connected=false and zero counts.
package ovshealth

import "github.com/b42labs/northwatch/internal/ovsdb/vs"

// errorStatKeys are the Interface.statistics counters whose non-zero value marks
// an interface as erroring: receive/transmit errors and drops.
var errorStatKeys = []string{"rx_errors", "tx_errors", "rx_dropped", "tx_dropped"}

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
	Members         []ChassisHealth `json:"members"`
}

// Aggregate sums the per-chassis caches into a FleetHealth. It preserves input
// order in Members and counts bridges, ports, interfaces and the down/erroring
// interfaces only for connected chassis, so an unreachable chassis is excluded
// from every total rather than counted as healthy. Unreachable is the number of
// chassis that are not connected.
func Aggregate(caches []ChassisCache) FleetHealth {
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
				if interfaceErroring(iface) {
					ch.ErrorInterfaces++
				}
			}
			fleet.Bridges += ch.Bridges
			fleet.Ports += ch.Ports
			fleet.Interfaces += ch.Interfaces
			fleet.DownInterfaces += ch.DownInterfaces
			fleet.ErrorInterfaces += ch.ErrorInterfaces
		}
		fleet.Members = append(fleet.Members, ch)
	}
	fleet.Unreachable = fleet.Chassis - fleet.Connected
	return fleet
}

// interfaceDown reports whether an interface's link_state is down. A nil
// link_state (absent from the OVSDB row) is not down.
func interfaceDown(iface vs.Interface) bool {
	return iface.LinkState != nil && *iface.LinkState == vs.InterfaceLinkStateDown
}

// interfaceErroring reports whether any of the interface's rx/tx error or drop
// counters is non-zero. A nil statistics map reads as all-zero, so it is not
// erroring.
func interfaceErroring(iface vs.Interface) bool {
	for _, key := range errorStatKeys {
		if iface.Statistics[key] > 0 {
			return true
		}
	}
	return false
}
