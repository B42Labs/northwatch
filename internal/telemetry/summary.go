package telemetry

import (
	"context"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// Querier provides telemetry data by querying libovsdb caches.
type Querier struct {
	NB client.Client
	SB client.Client
}

// NewQuerier creates a new telemetry querier.
func NewQuerier(nbClient, sbClient client.Client) *Querier {
	return &Querier{NB: nbClient, SB: sbClient}
}

// SummaryResult is the response for GET /api/v1/telemetry/summary.
type SummaryResult struct {
	Connected   map[string]bool    `json:"connected"`
	Counts      map[string]int     `json:"counts"`
	BFDStatus   map[string]int     `json:"bfd_status"`
	Propagation *PropagationResult `json:"propagation"`
}

// Summary returns an overview of connection status, entity counts, BFD status, and propagation.
func (q *Querier) Summary(ctx context.Context) (*SummaryResult, error) {
	result := &SummaryResult{
		Connected: map[string]bool{
			"nb": q.NB.Connected(),
			"sb": q.SB.Connected(),
		},
		Counts:  make(map[string]int),
		BFDStatus: make(map[string]int),
	}

	// Entity counts
	var lsw []nb.LogicalSwitch
	if err := q.NB.List(ctx, &lsw); err == nil {
		result.Counts["logical_switches"] = len(lsw)
	}
	var lsp []nb.LogicalSwitchPort
	if err := q.NB.List(ctx, &lsp); err == nil {
		result.Counts["logical_switch_ports"] = len(lsp)
	}
	var lr []nb.LogicalRouter
	if err := q.NB.List(ctx, &lr); err == nil {
		result.Counts["logical_routers"] = len(lr)
	}
	var lrp []nb.LogicalRouterPort
	if err := q.NB.List(ctx, &lrp); err == nil {
		result.Counts["logical_router_ports"] = len(lrp)
	}
	var acls []nb.ACL
	if err := q.NB.List(ctx, &acls); err == nil {
		result.Counts["acls"] = len(acls)
	}
	var chassis []sb.Chassis
	if err := q.SB.List(ctx, &chassis); err == nil {
		result.Counts["chassis"] = len(chassis)
	}
	var pb []sb.PortBinding
	if err := q.SB.List(ctx, &pb); err == nil {
		result.Counts["port_bindings"] = len(pb)
	}
	var lf []sb.LogicalFlow
	if err := q.SB.List(ctx, &lf); err == nil {
		result.Counts["logical_flows"] = len(lf)
	}

	// BFD status
	var bfdSessions []sb.BFD
	if err := q.SB.List(ctx, &bfdSessions); err == nil {
		for _, b := range bfdSessions {
			result.BFDStatus[b.Status]++
		}
	}

	// Propagation
	prop, err := q.Propagation(ctx)
	if err == nil {
		result.Propagation = prop
	}

	return result, nil
}

// FlowMetricsResult is the response for GET /api/v1/telemetry/flows.
type FlowMetricsResult struct {
	Total      int            `json:"total"`
	ByPipeline map[string]int `json:"by_pipeline"`
	ByTable    map[int]int    `json:"by_table"`
}

// FlowMetrics returns flow counts: total, by pipeline, by table.
func (q *Querier) FlowMetrics(ctx context.Context) (*FlowMetricsResult, error) {
	var flows []sb.LogicalFlow
	if err := q.SB.List(ctx, &flows); err != nil {
		return nil, err
	}

	result := &FlowMetricsResult{
		Total:      len(flows),
		ByPipeline: make(map[string]int),
		ByTable:    make(map[int]int),
	}
	for _, f := range flows {
		result.ByPipeline[f.Pipeline]++
		result.ByTable[f.TableID]++
	}
	return result, nil
}

// PropagationResult is the response for GET /api/v1/telemetry/propagation.
type PropagationResult struct {
	NbCfg   int                  `json:"nb_cfg"`
	SbNbCfg int                  `json:"sb_nb_cfg"`
	HvCfg   int                  `json:"hv_cfg"`
	Chassis []ChassisPropagation `json:"chassis"`
}

// ChassisPropagation shows per-chassis config realization status.
type ChassisPropagation struct {
	Name           string `json:"name"`
	Hostname       string `json:"hostname"`
	NbCfg          int    `json:"nb_cfg"`
	NbCfgTimestamp int    `json:"nb_cfg_timestamp,omitempty"`
	Lag            int    `json:"lag"`
}

// Propagation returns the NbCfg propagation chain.
func (q *Querier) Propagation(ctx context.Context) (*PropagationResult, error) {
	var nbGlobals []nb.NBGlobal
	if err := q.NB.List(ctx, &nbGlobals); err != nil {
		return nil, err
	}
	if len(nbGlobals) == 0 {
		return &PropagationResult{}, nil
	}
	g := nbGlobals[0]

	result := &PropagationResult{
		NbCfg: g.NbCfg,
		HvCfg: g.HvCfg,
	}

	var sbGlobals []sb.SBGlobal
	if err := q.SB.List(ctx, &sbGlobals); err == nil && len(sbGlobals) > 0 {
		result.SbNbCfg = sbGlobals[0].NbCfg
	}

	var chassisList []sb.Chassis
	if err := q.SB.List(ctx, &chassisList); err == nil {
		// Build a map of Chassis_Private for timestamps
		privTimestamps := map[string]int{}
		var privates []sb.ChassisPrivate
		if err := q.SB.List(ctx, &privates); err == nil {
			for _, p := range privates {
				privTimestamps[p.Name] = p.NbCfgTimestamp
			}
		}

		result.Chassis = make([]ChassisPropagation, 0, len(chassisList))
		for _, ch := range chassisList {
			cp := ChassisPropagation{
				Name:     ch.Name,
				Hostname: ch.Hostname,
				NbCfg:    ch.NbCfg,
				Lag:      g.NbCfg - ch.NbCfg,
			}
			if ts, ok := privTimestamps[ch.Name]; ok {
				cp.NbCfgTimestamp = ts
			}
			result.Chassis = append(result.Chassis, cp)
		}
	}

	return result, nil
}

// ClusterResult is the response for GET /api/v1/telemetry/cluster.
type ClusterResult struct {
	Connected   map[string]bool     `json:"connected"`
	Connections []ClusterConnection `json:"connections"`
}

// ClusterConnection represents an entry from the Connection table.
type ClusterConnection struct {
	Database    string            `json:"database"`
	Target      string            `json:"target"`
	IsConnected bool              `json:"is_connected"`
	ReadOnly    bool              `json:"read_only,omitempty"`
	Status      map[string]string `json:"status,omitempty"`
}

// Cluster returns cluster health: connection status and Connection table entries.
func (q *Querier) Cluster(ctx context.Context) (*ClusterResult, error) {
	result := &ClusterResult{
		Connected: map[string]bool{
			"nb": q.NB.Connected(),
			"sb": q.SB.Connected(),
		},
		Connections: []ClusterConnection{},
	}

	var nbConns []nb.Connection
	if err := q.NB.List(ctx, &nbConns); err == nil {
		for _, conn := range nbConns {
			result.Connections = append(result.Connections, ClusterConnection{
				Database:    "nb",
				Target:      conn.Target,
				IsConnected: conn.IsConnected,
				Status:      conn.Status,
			})
		}
	}

	var sbConns []sb.Connection
	if err := q.SB.List(ctx, &sbConns); err == nil {
		for _, conn := range sbConns {
			result.Connections = append(result.Connections, ClusterConnection{
				Database:    "sb",
				Target:      conn.Target,
				IsConnected: conn.IsConnected,
				ReadOnly:    conn.ReadOnly,
				Status:      conn.Status,
			})
		}
	}

	return result, nil
}
