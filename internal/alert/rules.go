package alert

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/inventory"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// StaleChassis returns a rule that fires when a chassis has not acknowledged the
// current nb_cfg generation within staleThreshold, deriving liveness from
// Chassis_Private.nb_cfg vs SB_Global.nb_cfg. Reading the deprecated
// Chassis.nb_cfg (as before) fired permanently-false warnings on OVN >= 20.06,
// where ovn-controller writes nb_cfg only to Chassis_Private.
func StaleChassis(sbClient client.Client, staleThreshold time.Duration) Rule {
	return Rule{
		Name:        "stale_chassis_config",
		Description: "Chassis has not acknowledged the current nb_cfg generation",
		Severity:    SeverityWarning,
		Check: func(ctx context.Context) ([]Alert, error) {
			var sbGlobals []sb.SBGlobal
			if err := sbClient.List(ctx, &sbGlobals); err != nil {
				return nil, fmt.Errorf("listing sb_global: %w", err)
			}
			if len(sbGlobals) == 0 {
				return nil, nil
			}
			sbNbCfg := sbGlobals[0].NbCfg

			var privates []sb.ChassisPrivate
			if err := sbClient.List(ctx, &privates); err != nil {
				return nil, fmt.Errorf("listing chassis_private: %w", err)
			}
			privByName := make(map[string]sb.ChassisPrivate, len(privates))
			for _, p := range privates {
				privByName[p.Name] = p
			}

			var chassisList []sb.Chassis
			if err := sbClient.List(ctx, &chassisList); err != nil {
				return nil, fmt.Errorf("listing chassis: %w", err)
			}

			now := time.Now()
			var alerts []Alert
			for _, ch := range chassisList {
				var priv *sb.ChassisPrivate
				if p, ok := privByName[ch.Name]; ok {
					priv = &p
				}
				lv := inventory.ComputeLiveness(priv, sbNbCfg, now, staleThreshold)
				// A missing Chassis_Private row means no ovn-controller heartbeat;
				// only flag it as stale when there is a generation to acknowledge.
				missing := priv == nil && sbNbCfg > 0
				if !lv.Stale && !missing {
					continue
				}

				var msg string
				if missing {
					msg = fmt.Sprintf("Chassis %s (%s) has no Chassis_Private row (no ovn-controller heartbeat)", ch.Name, ch.Hostname)
				} else {
					msg = fmt.Sprintf("Chassis %s (%s) is stale: acknowledged nb_cfg=%d, current=%d", ch.Name, ch.Hostname, lv.NbCfg, sbNbCfg)
				}
				alerts = append(alerts, Alert{
					Rule:     "stale_chassis_config",
					Severity: SeverityWarning,
					Message:  msg,
					Labels:   map[string]string{"chassis": ch.Name, "hostname": ch.Hostname},
				})
			}
			return alerts, nil
		},
	}
}

// PortDown returns a rule that fires when a non-virtual port binding has Up == false.
func PortDown(sbClient client.Client) Rule {
	return Rule{
		Name:        "port_down",
		Description: "Non-virtual port binding is down",
		Severity:    SeverityWarning,
		Check: func(ctx context.Context) ([]Alert, error) {
			var ports []sb.PortBinding
			if err := sbClient.List(ctx, &ports); err != nil {
				return nil, fmt.Errorf("listing port bindings: %w", err)
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
			return alerts, nil
		},
	}
}

// UnboundPort returns a rule that fires when a VIF port binding has no chassis assigned.
func UnboundPort(sbClient client.Client) Rule {
	return Rule{
		Name:        "unbound_port",
		Description: "VIF port binding has no chassis",
		Severity:    SeverityWarning,
		Check: func(ctx context.Context) ([]Alert, error) {
			var ports []sb.PortBinding
			if err := sbClient.List(ctx, &ports); err != nil {
				return nil, fmt.Errorf("listing port bindings: %w", err)
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
			return alerts, nil
		},
	}
}

// BFDDown returns a rule that fires when a BFD session status is "down".
func BFDDown(sbClient client.Client) Rule {
	return Rule{
		Name:        "bfd_down",
		Description: "BFD session is down",
		Severity:    SeverityCritical,
		Check: func(ctx context.Context) ([]Alert, error) {
			var sessions []sb.BFD
			if err := sbClient.List(ctx, &sessions); err != nil {
				return nil, fmt.Errorf("listing bfd sessions: %w", err)
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
			return alerts, nil
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
		Check: func(ctx context.Context) ([]Alert, error) {
			var flows []sb.LogicalFlow
			if err := sbClient.List(ctx, &flows); err != nil {
				return nil, fmt.Errorf("listing logical flows: %w", err)
			}
			count := len(flows)

			mu.Lock()
			defer mu.Unlock()

			// A zero count is indistinguishable from a purged cache or a
			// monitoring gap (e.g. Logical_Flow is skipped, or a snapshot session
			// suspended live), so do not treat it as a 100% drop and do not update
			// the baseline off it — otherwise repopulation would fire too.
			if count == 0 {
				return nil, nil
			}

			if firstRun {
				lastCount = count
				firstRun = false
				return nil, nil
			}

			if lastCount == 0 {
				lastCount = count
				return nil, nil
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
				}}, nil
			}
			return nil, nil
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
		Check: func(ctx context.Context) ([]Alert, error) {
			var groups []sb.HAChassisGroup
			if err := sbClient.List(ctx, &groups); err != nil {
				return nil, fmt.Errorf("listing ha_chassis_group: %w", err)
			}

			var haChassisList []sb.HAChassis
			if err := sbClient.List(ctx, &haChassisList); err != nil {
				return nil, fmt.Errorf("listing ha_chassis: %w", err)
			}

			var chassisList []sb.Chassis
			if err := sbClient.List(ctx, &chassisList); err != nil {
				return nil, fmt.Errorf("listing chassis: %w", err)
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
				return nil, nil
			}

			var alerts []Alert
			for groupUUID, currentChassis := range currentActive {
				prevChassis, existed := lastActive[groupUUID]
				if existed && prevChassis != currentChassis {
					// Failover detected
					groupName := shortID(groupUUID)
					for _, g := range groups {
						if g.UUID == groupUUID {
							groupName = g.Name
							break
						}
					}
					prevName := chassisNames[prevChassis]
					if prevName == "" {
						prevName = shortID(prevChassis)
					}
					currentName := chassisNames[currentChassis]
					if currentName == "" {
						currentName = shortID(currentChassis)
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
			return alerts, nil
		},
	}
}

func portType(t string) string {
	if t == "" {
		return "VIF"
	}
	return t
}

// shortID returns the first 8 runes of an identifier for display, guarding
// against strings shorter than 8 (a raw s[:8] panics on those).
func shortID(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return s
	}
	return string(r[:8])
}
