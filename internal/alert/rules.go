package alert

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// StaleChassis returns a rule that fires when a chassis NbCfg lags behind NB_Global.NbCfg
// by more than threshold generations.
func StaleChassis(nbClient, sbClient client.Client, threshold int) Rule {
	return Rule{
		Name:        "stale_chassis_config",
		Description: fmt.Sprintf("Chassis NbCfg lags behind NB_Global by more than %d", threshold),
		Severity:    SeverityWarning,
		Check: func(ctx context.Context) []Alert {
			var nbGlobals []nb.NBGlobal
			if err := nbClient.List(ctx, &nbGlobals); err != nil || len(nbGlobals) == 0 {
				return nil
			}
			nbCfg := nbGlobals[0].NbCfg

			var chassisList []sb.Chassis
			if err := sbClient.List(ctx, &chassisList); err != nil {
				return nil
			}

			var alerts []Alert
			for _, ch := range chassisList {
				lag := nbCfg - ch.NbCfg
				if lag > threshold {
					alerts = append(alerts, Alert{
						Rule:     "stale_chassis_config",
						Severity: SeverityWarning,
						Message:  fmt.Sprintf("Chassis %s (%s) is %d generations behind (nb_cfg=%d, chassis_nb_cfg=%d)", ch.Name, ch.Hostname, lag, nbCfg, ch.NbCfg),
						Labels:   map[string]string{"chassis": ch.Name, "hostname": ch.Hostname},
					})
				}
			}
			return alerts
		},
	}
}

// PortDown returns a rule that fires when a non-virtual port binding has Up == false.
func PortDown(sbClient client.Client) Rule {
	return Rule{
		Name:        "port_down",
		Description: "Non-virtual port binding is down",
		Severity:    SeverityWarning,
		Check: func(ctx context.Context) []Alert {
			var ports []sb.PortBinding
			if err := sbClient.List(ctx, &ports); err != nil {
				return nil
			}

			var alerts []Alert
			for _, p := range ports {
				if p.Type == "virtual" {
					continue
				}
				if p.Up != nil && !*p.Up {
					alerts = append(alerts, Alert{
						Rule:     "port_down",
						Severity: SeverityWarning,
						Message:  fmt.Sprintf("Port %s is down", p.LogicalPort),
						Labels:   map[string]string{"port": p.LogicalPort, "type": portType(p.Type)},
					})
				}
			}
			return alerts
		},
	}
}

// UnboundPort returns a rule that fires when a VIF port binding has no chassis assigned.
func UnboundPort(sbClient client.Client) Rule {
	return Rule{
		Name:        "unbound_port",
		Description: "VIF port binding has no chassis",
		Severity:    SeverityWarning,
		Check: func(ctx context.Context) []Alert {
			var ports []sb.PortBinding
			if err := sbClient.List(ctx, &ports); err != nil {
				return nil
			}

			var alerts []Alert
			for _, p := range ports {
				// Only check VIF ports (type == "")
				if p.Type != "" {
					continue
				}
				if p.Chassis == nil || *p.Chassis == "" {
					alerts = append(alerts, Alert{
						Rule:     "unbound_port",
						Severity: SeverityWarning,
						Message:  fmt.Sprintf("VIF port %s has no chassis binding", p.LogicalPort),
						Labels:   map[string]string{"port": p.LogicalPort},
					})
				}
			}
			return alerts
		},
	}
}

// BFDDown returns a rule that fires when a BFD session status is "down".
func BFDDown(sbClient client.Client) Rule {
	return Rule{
		Name:        "bfd_down",
		Description: "BFD session is down",
		Severity:    SeverityCritical,
		Check: func(ctx context.Context) []Alert {
			var sessions []sb.BFD
			if err := sbClient.List(ctx, &sessions); err != nil {
				return nil
			}

			var alerts []Alert
			for _, s := range sessions {
				if s.Status == sb.BFDStatusDown {
					alerts = append(alerts, Alert{
						Rule:     "bfd_down",
						Severity: SeverityCritical,
						Message:  fmt.Sprintf("BFD session to %s on %s is down", s.DstIP, s.LogicalPort),
						Labels:   map[string]string{"dst_ip": s.DstIP, "logical_port": s.LogicalPort, "chassis": s.ChassisName},
					})
				}
			}
			return alerts
		},
	}
}

// FlowCountAnomaly returns a rule that fires when the logical flow count changes
// by more than thresholdPct percent since the last check.
// NOTE: The returned Rule captures mutable state (lastCount) in the Check closure.
// Do not register the same Rule value in multiple engines.
func FlowCountAnomaly(sbClient client.Client, thresholdPct float64) Rule {
	var (
		mu        sync.Mutex
		lastCount int
		firstRun  = true
	)

	return Rule{
		Name:        "flow_count_anomaly",
		Description: fmt.Sprintf("Logical flow count changed by more than %.0f%%", thresholdPct),
		Severity:    SeverityWarning,
		Check: func(ctx context.Context) []Alert {
			var flows []sb.LogicalFlow
			if err := sbClient.List(ctx, &flows); err != nil {
				return nil
			}
			count := len(flows)

			mu.Lock()
			defer mu.Unlock()

			if firstRun {
				lastCount = count
				firstRun = false
				return nil
			}

			if lastCount == 0 {
				lastCount = count
				return nil
			}

			pctChange := math.Abs(float64(count-lastCount)) / float64(lastCount) * 100
			prev := lastCount
			lastCount = count

			if pctChange > thresholdPct {
				return []Alert{{
					Rule:     "flow_count_anomaly",
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("Logical flow count changed by %.1f%% (%d → %d)", pctChange, prev, count),
					Labels:   map[string]string{},
				}}
			}
			return nil
		},
	}
}

// HAFailover returns a rule that detects when the active chassis in an HA chassis
// group changes, indicating a gateway failover event.
func HAFailover(sbClient client.Client) Rule {
	var (
		mu          sync.Mutex
		lastActive  map[string]string // HA group UUID -> active chassis UUID
		initialized bool
	)

	return Rule{
		Name:        "ha_failover",
		Description: "HA chassis group active chassis changed (gateway failover)",
		Severity:    SeverityCritical,
		Check: func(ctx context.Context) []Alert {
			var groups []sb.HAChassisGroup
			if err := sbClient.List(ctx, &groups); err != nil {
				return nil
			}

			var haChassisList []sb.HAChassis
			if err := sbClient.List(ctx, &haChassisList); err != nil {
				return nil
			}

			var chassisList []sb.Chassis
			if err := sbClient.List(ctx, &chassisList); err != nil {
				return nil
			}

			// Build chassis name lookup
			chassisNames := make(map[string]string, len(chassisList))
			for _, ch := range chassisList {
				chassisNames[ch.UUID] = ch.Name
			}

			// Build HAChassis lookup by UUID
			haChassisMap := make(map[string]sb.HAChassis, len(haChassisList))
			for _, hac := range haChassisList {
				haChassisMap[hac.UUID] = hac
			}

			// Determine current active chassis per group (highest priority)
			currentActive := make(map[string]string) // group UUID -> chassis UUID
			for _, group := range groups {
				var bestPriority int
				var bestChassis string
				for _, hacUUID := range group.HaChassis {
					hac, ok := haChassisMap[hacUUID]
					if !ok {
						continue
					}
					if hac.Chassis == nil || *hac.Chassis == "" {
						continue
					}
					if hac.Priority > bestPriority || bestChassis == "" {
						bestPriority = hac.Priority
						bestChassis = *hac.Chassis
					}
				}
				if bestChassis != "" {
					currentActive[group.UUID] = bestChassis
				}
			}

			mu.Lock()
			defer mu.Unlock()

			if !initialized {
				lastActive = currentActive
				initialized = true
				return nil
			}

			var alerts []Alert
			for groupUUID, currentChassis := range currentActive {
				prevChassis, existed := lastActive[groupUUID]
				if existed && prevChassis != currentChassis {
					// Failover detected
					groupName := groupUUID[:8]
					for _, g := range groups {
						if g.UUID == groupUUID {
							groupName = g.Name
							break
						}
					}
					prevName := chassisNames[prevChassis]
					if prevName == "" {
						prevName = prevChassis[:8]
					}
					currentName := chassisNames[currentChassis]
					if currentName == "" {
						currentName = currentChassis[:8]
					}

					alerts = append(alerts, Alert{
						Rule:     "ha_failover",
						Severity: SeverityCritical,
						Message:  fmt.Sprintf("HA group %s failover: %s -> %s", groupName, prevName, currentName),
						Labels: map[string]string{
							"ha_group":         groupUUID,
							"ha_group_name":    groupName,
							"previous_chassis": prevName,
							"current_chassis":  currentName,
						},
					})
				}
			}

			lastActive = currentActive
			return alerts
		},
	}
}

func portType(t string) string {
	if t == "" {
		return "VIF"
	}
	return t
}
