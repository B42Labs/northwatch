package telemetry

import (
	"context"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

var (
	descConnected = prometheus.NewDesc(
		"northwatch_ovsdb_connected",
		"Whether the OVSDB client is connected (1=yes, 0=no).",
		[]string{"database"}, nil,
	)
	descTableRows = prometheus.NewDesc(
		"northwatch_ovsdb_table_rows",
		"Number of rows in an OVSDB table.",
		[]string{"database", "table"}, nil,
	)
	descNbCfg = prometheus.NewDesc(
		"northwatch_nb_cfg",
		"NB_Global.NbCfg — source-of-truth configuration generation.",
		nil, nil,
	)
	descSbCfg = prometheus.NewDesc(
		"northwatch_sb_cfg",
		"NB_Global.SbCfg — configuration generation applied by northd.",
		nil, nil,
	)
	descHvCfg = prometheus.NewDesc(
		"northwatch_hv_cfg",
		"NB_Global.HvCfg — configuration generation realized by all hypervisors.",
		nil, nil,
	)
	descSbNbCfg = prometheus.NewDesc(
		"northwatch_sb_nb_cfg",
		"SB_Global.NbCfg — configuration generation in the southbound database.",
		nil, nil,
	)
	descChassisNbCfg = prometheus.NewDesc(
		"northwatch_chassis_nb_cfg",
		"Chassis NbCfg — configuration generation realized by chassis.",
		[]string{"chassis", "hostname"}, nil,
	)
	descChassisNbCfgLag = prometheus.NewDesc(
		"northwatch_chassis_nb_cfg_lag",
		"NB_Global.NbCfg minus Chassis.NbCfg — config propagation lag.",
		[]string{"chassis", "hostname"}, nil,
	)
	descLogicalFlows = prometheus.NewDesc(
		"northwatch_logical_flows_total",
		"Number of logical flows by pipeline.",
		[]string{"pipeline"}, nil,
	)
	descPortBindings = prometheus.NewDesc(
		"northwatch_port_bindings_total",
		"Number of port bindings by type.",
		[]string{"type"}, nil,
	)
	descChassisPortCount = prometheus.NewDesc(
		"northwatch_chassis_port_count",
		"Number of port bindings per chassis.",
		[]string{"chassis", "hostname"}, nil,
	)
	descBFDSessions = prometheus.NewDesc(
		"northwatch_bfd_sessions",
		"Number of BFD sessions by status.",
		[]string{"status"}, nil,
	)
	descOVSDBConnections = prometheus.NewDesc(
		"northwatch_ovsdb_connections",
		"OVSDB Connection table entries.",
		[]string{"database", "connected"}, nil,
	)
)

// Collector implements prometheus.Collector, querying libovsdb caches on each scrape.
type Collector struct {
	NB client.Client
	SB client.Client
}

// NewCollector creates a new Prometheus metrics collector.
func NewCollector(nbClient, sbClient client.Client) *Collector {
	return &Collector{NB: nbClient, SB: sbClient}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descConnected
	ch <- descTableRows
	ch <- descNbCfg
	ch <- descSbCfg
	ch <- descHvCfg
	ch <- descSbNbCfg
	ch <- descChassisNbCfg
	ch <- descChassisNbCfgLag
	ch <- descLogicalFlows
	ch <- descPortBindings
	ch <- descChassisPortCount
	ch <- descBFDSessions
	ch <- descOVSDBConnections
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	// The prometheus.Collector interface does not provide a request context, so
	// per-scrape cancellation cannot be propagated. All operations below are
	// fast in-memory libovsdb cache reads, so context.Background() is acceptable.
	ctx := context.Background()

	// Connection status
	ch <- gauge(descConnected, boolToFloat(c.NB.Connected()), "nb")
	ch <- gauge(descConnected, boolToFloat(c.SB.Connected()), "sb")

	// Fetch the chassis list once and reuse it for lag and per-chassis port counts.
	var chassisList []sb.Chassis
	chassisListErr := c.SB.List(ctx, &chassisList)

	// NB_Global metrics
	var nbGlobals []nb.NBGlobal
	if err := c.NB.List(ctx, &nbGlobals); err == nil && len(nbGlobals) > 0 {
		g := nbGlobals[0]
		ch <- gauge(descNbCfg, float64(g.NbCfg))
		ch <- gauge(descSbCfg, float64(g.SbCfg))
		ch <- gauge(descHvCfg, float64(g.HvCfg))

		// NB table row counts
		c.collectNBTableRows(ctx, ch)

		// Per-chassis NbCfg and lag
		if chassisListErr == nil {
			for _, ch2 := range chassisList {
				ch <- gauge(descChassisNbCfg, float64(ch2.NbCfg), ch2.Name, ch2.Hostname)
				ch <- gauge(descChassisNbCfgLag, float64(g.NbCfg-ch2.NbCfg), ch2.Name, ch2.Hostname)
			}
		}
	}

	// SB_Global metrics
	var sbGlobals []sb.SBGlobal
	if err := c.SB.List(ctx, &sbGlobals); err == nil && len(sbGlobals) > 0 {
		ch <- gauge(descSbNbCfg, float64(sbGlobals[0].NbCfg))
	}

	// SB table row counts
	c.collectSBTableRows(ctx, ch)

	// Logical flows by pipeline
	var flows []sb.LogicalFlow
	if err := c.SB.List(ctx, &flows); err == nil {
		pipelines := map[string]int{}
		for _, f := range flows {
			pipelines[f.Pipeline]++
		}
		for p, count := range pipelines {
			ch <- gauge(descLogicalFlows, float64(count), p)
		}
	}

	// Port bindings by type
	var ports []sb.PortBinding
	if err := c.SB.List(ctx, &ports); err == nil {
		// Build a chassis UUID-to-info map to avoid N+1 lookups, reusing
		// the chassis list fetched at the top of Collect.
		chassisMap := map[string]*sb.Chassis{}
		if chassisListErr == nil {
			for i := range chassisList {
				chassisMap[chassisList[i].UUID] = &chassisList[i]
			}
		}

		types := map[string]int{}
		chassisPorts := map[string]int{}
		chassisHostnames := map[string]string{}
		for _, p := range ports {
			t := p.Type
			if t == "" {
				t = "VIF"
			}
			types[t]++
			if p.Chassis != nil && *p.Chassis != "" {
				if ch, ok := chassisMap[*p.Chassis]; ok {
					chassisPorts[ch.Name]++
					chassisHostnames[ch.Name] = ch.Hostname
				}
			}
		}
		for t, count := range types {
			ch <- gauge(descPortBindings, float64(count), t)
		}
		for name, count := range chassisPorts {
			ch <- gauge(descChassisPortCount, float64(count), name, chassisHostnames[name])
		}
	}

	// BFD sessions by status
	var bfdSessions []sb.BFD
	if err := c.SB.List(ctx, &bfdSessions); err == nil {
		statuses := map[string]int{}
		for _, b := range bfdSessions {
			statuses[b.Status]++
		}
		for s, count := range statuses {
			ch <- gauge(descBFDSessions, float64(count), s)
		}
	}

	// Connection table entries
	c.collectConnections(ctx, ch)
}

func (c *Collector) collectNBTableRows(ctx context.Context, ch chan<- prometheus.Metric) {
	listCount := func(table string, result any) {
		if err := c.NB.List(ctx, result); err == nil {
			ch <- gauge(descTableRows, float64(listLen(result)), "nb", table)
		}
	}
	var lsw []nb.LogicalSwitch
	listCount("Logical_Switch", &lsw)
	var lsp []nb.LogicalSwitchPort
	listCount("Logical_Switch_Port", &lsp)
	var lr []nb.LogicalRouter
	listCount("Logical_Router", &lr)
	var lrp []nb.LogicalRouterPort
	listCount("Logical_Router_Port", &lrp)
	var acls []nb.ACL
	listCount("ACL", &acls)
	var nats []nb.NAT
	listCount("NAT", &nats)
	var lbs []nb.LoadBalancer
	listCount("Load_Balancer", &lbs)
}

func (c *Collector) collectSBTableRows(ctx context.Context, ch chan<- prometheus.Metric) {
	listCount := func(table string, result any) {
		if err := c.SB.List(ctx, result); err == nil {
			ch <- gauge(descTableRows, float64(listLen(result)), "sb", table)
		}
	}
	var chassis []sb.Chassis
	listCount("Chassis", &chassis)
	var pb []sb.PortBinding
	listCount("Port_Binding", &pb)
	var lf []sb.LogicalFlow
	listCount("Logical_Flow", &lf)
	var db []sb.DatapathBinding
	listCount("Datapath_Binding", &db)
}

func (c *Collector) collectConnections(ctx context.Context, ch chan<- prometheus.Metric) {
	var nbConns []nb.Connection
	if err := c.NB.List(ctx, &nbConns); err == nil {
		connected, disconnected := 0, 0
		for _, conn := range nbConns {
			if conn.IsConnected {
				connected++
			} else {
				disconnected++
			}
		}
		ch <- gauge(descOVSDBConnections, float64(connected), "nb", "true")
		ch <- gauge(descOVSDBConnections, float64(disconnected), "nb", "false")
	}

	var sbConns []sb.Connection
	if err := c.SB.List(ctx, &sbConns); err == nil {
		connected, disconnected := 0, 0
		for _, conn := range sbConns {
			if conn.IsConnected {
				connected++
			} else {
				disconnected++
			}
		}
		ch <- gauge(descOVSDBConnections, float64(connected), "sb", "true")
		ch <- gauge(descOVSDBConnections, float64(disconnected), "sb", "false")
	}
}

func gauge(desc *prometheus.Desc, value float64, labels ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labels...)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func listLen(result any) int {
	// result is a *[]T; use reflect-free approach via type switch on known types.
	// For simplicity, we use a helper that counts after List() already populated the slice.
	switch v := result.(type) {
	case *[]nb.LogicalSwitch:
		return len(*v)
	case *[]nb.LogicalSwitchPort:
		return len(*v)
	case *[]nb.LogicalRouter:
		return len(*v)
	case *[]nb.LogicalRouterPort:
		return len(*v)
	case *[]nb.ACL:
		return len(*v)
	case *[]nb.NAT:
		return len(*v)
	case *[]nb.LoadBalancer:
		return len(*v)
	case *[]sb.Chassis:
		return len(*v)
	case *[]sb.PortBinding:
		return len(*v)
	case *[]sb.LogicalFlow:
		return len(*v)
	case *[]sb.DatapathBinding:
		return len(*v)
	default:
		return 0
	}
}
