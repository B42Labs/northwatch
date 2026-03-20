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

func portType(t string) string {
	if t == "" {
		return "VIF"
	}
	return t
}
