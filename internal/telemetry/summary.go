package telemetry

import (
	"context"
	"sort"
	"strings"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb/serverdb"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// OVSDB database names as they appear in the "_Server" Database table's name
// column. Used to pick the relevant Raft row out of the server's databases.
const (
	nbDatabaseName = "OVN_Northbound"
	sbDatabaseName = "OVN_Southbound"
)

// Querier provides telemetry data by querying libovsdb caches.
type Querier struct {
	NB client.Client
	SB client.Client

	// NBServers and SBServers are optional per-endpoint "_Server" monitors (see
	// ovsdb.OVNDatabases). Each reports one cluster member's Raft view;
	// RaftHealth aggregates them into a member list. Empty slices mean Raft state
	// is unavailable.
	NBServers []ovndb.ServerMonitor
	SBServers []ovndb.ServerMonitor
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

// RaftHealthResult reports Raft cluster health plus Northwatch's own client
// connection state for both the Northbound and Southbound databases.
type RaftHealthResult struct {
	NB RaftDBHealth `json:"nb"`
	SB RaftDBHealth `json:"sb"`
}

// RaftDBHealth describes one OVSDB database from two angles: whether
// Northwatch's client is connected to it, and the real Raft cluster state
// aggregated from the "_Server" database of each configured endpoint.
type RaftDBHealth struct {
	// ClientConnected reports whether Northwatch's libovsdb client currently has
	// a live connection to this database. This is independent of Raft state — it
	// answers "can Northwatch reach OVN?", not "is the cluster healthy?".
	ClientConnected bool `json:"client_connected"`

	// Cluster is the Raft cluster state aggregated across the configured
	// endpoints. Cluster.Available is false when no member could be read.
	Cluster RaftCluster `json:"cluster"`

	// Listeners are the rows of this database's Connection table — the
	// configured connection methods (e.g. passive ptcp:/pssl: listeners). For a
	// passive listener IsConnected is typically false; the meaningful liveness
	// lives in Status (bound_port, n_connections), not IsConnected.
	Listeners []ConnectionDetail `json:"listeners"`
}

// RaftCluster is the aggregated Raft view of one database, built from the
// per-endpoint "_Server" Database rows.
type RaftCluster struct {
	// Available is true when at least one member's "_Server" row was read. When
	// false the UI should show the cluster view as "unavailable" rather than
	// "unhealthy" (e.g. standalone server or offline snapshot).
	Available bool `json:"available"`
	// Model is the OVSDB service model reported by members: "standalone",
	// "clustered", or "relay". Only "clustered" databases run Raft.
	Model string `json:"model,omitempty"`
	// ClusterID (cid) identifies the Raft cluster. All members should agree;
	// SplitBrain is set if reachable members report differing cluster IDs.
	ClusterID  string `json:"cluster_id,omitempty"`
	SplitBrain bool   `json:"split_brain,omitempty"`
	// Total is the number of configured endpoints; Reachable is how many
	// reported a "_Server" row. These reflect the endpoints Northwatch was
	// configured with, which may be a subset of the real cluster.
	Total     int `json:"total"`
	Reachable int `json:"reachable"`
	// HasLeader is true when exactly one reachable member reports itself leader;
	// LeaderID is that member's server ID.
	HasLeader bool   `json:"has_leader"`
	LeaderID  string `json:"leader_id,omitempty"`
	// Members is one entry per configured endpoint, reachable or not.
	Members []RaftMember `json:"members"`
}

// RaftMember is one cluster member's self-reported Raft state, read from the
// "_Server" Database row on its endpoint.
type RaftMember struct {
	// Endpoint is the configured address of this member.
	Endpoint string `json:"endpoint"`
	// Reachable is true when this member's "_Server" row was read.
	Reachable bool `json:"reachable"`
	// ServerID (sid) identifies this member within the cluster.
	ServerID string `json:"server_id,omitempty"`
	// Leader is true when this member reports itself as the current Raft leader.
	Leader bool `json:"leader"`
	// Connected reports whether this member is in contact with the cluster
	// (part of the quorum). False means partitioned or still catching up.
	Connected bool `json:"connected"`
	// Index is this member's Raft log index — its position in the replicated log.
	Index int `json:"index,omitempty"`
}

// ConnectionDetail describes a single OVSDB Connection table row.
type ConnectionDetail struct {
	UUID        string `json:"uuid"`
	Target      string `json:"target"`
	IsConnected bool   `json:"is_connected"`
	Status      string `json:"status,omitempty"`
}

// RaftHealth returns Raft cluster health plus client-connection state for NB and SB.
func (q *Querier) RaftHealth(ctx context.Context) (*RaftHealthResult, error) {
	result := &RaftHealthResult{}

	result.NB.ClientConnected = q.NB.Connected()
	var nbConns []nb.Connection
	if err := q.NB.List(ctx, &nbConns); err == nil {
		for _, c := range nbConns {
			result.NB.Listeners = append(result.NB.Listeners, ConnectionDetail{
				UUID:        c.UUID,
				Target:      c.Target,
				IsConnected: c.IsConnected,
				Status:      formatStatus(c.Status),
			})
		}
	}
	result.NB.Cluster = clusterStatus(ctx, q.NBServers, nbDatabaseName)

	result.SB.ClientConnected = q.SB.Connected()
	var sbConns []sb.Connection
	if err := q.SB.List(ctx, &sbConns); err == nil {
		for _, c := range sbConns {
			result.SB.Listeners = append(result.SB.Listeners, ConnectionDetail{
				UUID:        c.UUID,
				Target:      c.Target,
				IsConnected: c.IsConnected,
				Status:      formatStatus(c.Status),
			})
		}
	}
	result.SB.Cluster = clusterStatus(ctx, q.SBServers, sbDatabaseName)

	if result.NB.Listeners == nil {
		result.NB.Listeners = []ConnectionDetail{}
	}
	if result.SB.Listeners == nil {
		result.SB.Listeners = []ConnectionDetail{}
	}

	return result, nil
}

// clusterStatus aggregates the per-endpoint "_Server" monitors into a cluster
// view for dbName. Each monitor reports only its own server's Raft row, so the
// member list is reconstructed by reading dbName's row from every endpoint.
// Unreachable endpoints are still listed (Reachable=false).
func clusterStatus(ctx context.Context, monitors []ovndb.ServerMonitor, dbName string) RaftCluster {
	c := RaftCluster{Members: []RaftMember{}, Total: len(monitors)}
	leaders := 0
	for _, mon := range monitors {
		member := RaftMember{Endpoint: mon.Endpoint}
		if d, ok := readDatabaseRow(ctx, mon.Client, dbName); ok {
			member.Reachable = true
			member.Leader = d.Leader
			member.Connected = d.Connected
			if d.Sid != nil {
				member.ServerID = *d.Sid
			}
			if d.Index != nil {
				member.Index = *d.Index
			}

			c.Available = true
			c.Reachable++
			if c.Model == "" {
				c.Model = d.Model
			}
			if d.Cid != nil {
				if c.ClusterID == "" {
					c.ClusterID = *d.Cid
				} else if c.ClusterID != *d.Cid {
					c.SplitBrain = true
				}
			}
			if d.Leader {
				leaders++
				c.LeaderID = member.ServerID
			}
		}
		c.Members = append(c.Members, member)
	}
	// Only claim a leader when exactly one member reports leadership; two
	// leaders means a split we should surface, not paper over.
	c.HasLeader = leaders == 1
	if leaders > 1 {
		c.SplitBrain = true
		c.LeaderID = ""
	}
	return c
}

// readDatabaseRow reads the "_Server" Database row for dbName from a single
// member monitor. It returns ok=false when the client is nil, disconnected,
// errors, or has no matching row.
func readDatabaseRow(ctx context.Context, server client.Client, dbName string) (serverdb.Database, bool) {
	if server == nil || !server.Connected() {
		return serverdb.Database{}, false
	}
	var dbs []serverdb.Database
	if err := server.List(ctx, &dbs); err != nil {
		return serverdb.Database{}, false
	}
	for _, d := range dbs {
		if d.Name == dbName {
			return d, true
		}
	}
	return serverdb.Database{}, false
}

// formatStatus renders an OVSDB status map as a stable, comma-separated
// "key=value" string with keys sorted, so the full status (e.g. bound_port,
// n_connections, sec_since_connect) is shown rather than a single arbitrary key.
func formatStatus(status map[string]string) string {
	if len(status) == 0 {
		return ""
	}
	keys := make([]string, 0, len(status))
	for k := range status {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+status[k])
	}
	return strings.Join(parts, ", ")
}
