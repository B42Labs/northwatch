package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/b42labs/northwatch/internal/alert"
	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/api/handler"
	"github.com/b42labs/northwatch/internal/cluster"
	"github.com/b42labs/northwatch/internal/config"
	"github.com/b42labs/northwatch/internal/correlate"
	"github.com/b42labs/northwatch/internal/debug"
	"github.com/b42labs/northwatch/internal/enrich"
	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/flowdiff"
	"github.com/b42labs/northwatch/internal/history"
	"github.com/b42labs/northwatch/internal/impact"
	"github.com/b42labs/northwatch/internal/openapi"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/b42labs/northwatch/internal/search"
	"github.com/b42labs/northwatch/internal/snapshot"
	"github.com/b42labs/northwatch/internal/snapshotsession"
	"github.com/b42labs/northwatch/internal/telemetry"
	"github.com/b42labs/northwatch/internal/write"
	northwatchUI "github.com/b42labs/northwatch/ui"
)

func main() {
	// Stage 1 of the offline workflow: "northwatch snapshot" captures the live
	// NB/SB databases to a file. Without the subcommand we run the server.
	if len(os.Args) > 1 && os.Args[1] == "snapshot" {
		if err := runSnapshot(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runSnapshot implements the "northwatch snapshot" subcommand: it connects once
// to the live OVN NB/SB databases and writes their full contents to a file that
// can later be served offline via "northwatch --snapshot <file>".
func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("northwatch snapshot", flag.ContinueOnError)
	nbAddr := fs.String("ovn-nb-addr", os.Getenv("NORTHWATCH_OVN_NB_ADDR"), "OVN Northbound DB address, comma-separated for failover")
	sbAddr := fs.String("ovn-sb-addr", os.Getenv("NORTHWATCH_OVN_SB_ADDR"), "OVN Southbound DB address, comma-separated for failover")
	out := "northwatch-snapshot.json"
	fs.StringVar(&out, "output", out, "Output file path for the snapshot")
	fs.StringVar(&out, "o", out, "Output file path for the snapshot (shorthand)")
	// Same initial-monitor tuning as the server, so capturing a snapshot from a
	// huge deployment doesn't overload the OVN databases in one request. Capture
	// is a live connection, so it shares the server's staged default.
	defBatchDelay := config.DefaultMonitorBatchDelay
	if v := os.Getenv("NORTHWATCH_MONITOR_BATCH_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			defBatchDelay = d
		}
	}
	monitorBatchDelay := fs.Duration("monitor-batch-delay", defBatchDelay, "Delay between staged per-table monitor requests (e.g. 100ms, 1s); 0 loads all tables in a single request")
	monitorSkipTables := fs.String("monitor-skip-tables", os.Getenv("NORTHWATCH_MONITOR_SKIP_TABLES"), "Comma-separated OVN table names to never capture (e.g. Logical_Flow,MAC_Binding,FDB)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nbAddr == "" {
		return fmt.Errorf("--ovn-nb-addr is required (or set NORTHWATCH_OVN_NB_ADDR)")
	}
	if *sbAddr == "" {
		return fmt.Errorf("--ovn-sb-addr is required (or set NORTHWATCH_OVN_SB_ADDR)")
	}

	nbModel, err := nb.FullDatabaseModel()
	if err != nil {
		return fmt.Errorf("creating NB model: %w", err)
	}
	sbModel, err := sb.FullDatabaseModel()
	if err != nil {
		return fmt.Errorf("creating SB model: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("Connecting to OVN databases...")
	mon := ovndb.MonitorOptions{
		BatchDelay: *monitorBatchDelay,
		SkipTables: config.SplitCSV(*monitorSkipTables),
	}
	dbs, err := ovndb.Connect(ctx, *nbAddr, *sbAddr, nbModel, sbModel, mon)
	if err != nil {
		return fmt.Errorf("connecting to OVN: %w", err)
	}
	defer dbs.Close()

	fmt.Println("Connected; capturing snapshot...")
	snap, err := snapshot.Capture(dbs.NB, dbs.SB, nb.Schema().Name, sb.Schema().Name)
	if err != nil {
		return fmt.Errorf("capturing snapshot: %w", err)
	}
	snap.Source = &snapshot.Source{NBAddr: *nbAddr, SBAddr: *sbAddr}

	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("creating %s: %w", out, err)
	}
	defer func() { _ = f.Close() }()
	if err := snapshot.Save(snap, f); err != nil {
		return fmt.Errorf("writing snapshot: %w", err)
	}

	fmt.Printf("Snapshot written to %s (NB: %d rows, SB: %d rows)\n", out, snap.NB.RowCount(), snap.SB.RowCount())
	return nil
}

func run() error {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	// Stage 2 of the offline workflow: when --snapshot is set, spin up local
	// in-memory OVSDB servers from the file and point the cluster at them, so
	// every downstream subsystem runs unchanged against the offline copy.
	var snapInfo *handler.SnapshotInfo
	if cfg.SnapshotFile != "" {
		info, closeSnapshot, err := setupSnapshotMode(cfg)
		if err != nil {
			return err
		}
		defer closeSnapshot()
		snapInfo = info
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build cluster registry from config
	reg := cluster.NewRegistry()
	var stopFuncs []func()

	for _, cc := range cfg.Clusters {
		c, clusterStops, err := buildCluster(ctx, cfg, cc)
		if err != nil {
			reg.Close()
			return err
		}
		stopFuncs = append(stopFuncs, clusterStops...)
		reg.Register(cc.Name, c)
	}
	defer reg.Close()
	defer func() {
		for _, stop := range stopFuncs {
			stop()
		}
	}()

	def := reg.Default()

	// Per-chassis OVS visibility (opt-in, default cluster only). Built only when
	// the system-id → mgmt-addr mapping is set; one monitored connection per
	// chassis, isolated from NB/SB and from each other.
	var ovsPool *ovndb.OVSPool
	if len(cfg.OVSMgmtAddrs) > 0 {
		vsModel, err := vs.FullDatabaseModel()
		if err != nil {
			return fmt.Errorf("creating OVS model: %w", err)
		}
		tlsConfig, err := ovndb.BuildTLSConfig(cfg.OVSTLSCert, cfg.OVSTLSKey, cfg.OVSTLSCA)
		if err != nil {
			return fmt.Errorf("building OVS TLS config: %w", err)
		}
		ovsPool = ovndb.ConnectOVSPool(vsModel, tlsConfig, cfg.OVSMgmtAddrs)
		defer ovsPool.Close()
		fmt.Printf("OVS visibility enabled for %d chassis\n", len(cfg.OVSMgmtAddrs))
	}

	// History & snapshot store (shared across clusters, uses default cluster)
	historyStore, err := history.NewStore(cfg.HistoryDBPath)
	if err != nil {
		return fmt.Errorf("opening history database: %w", err)
	}
	defer func() { _ = historyStore.Close() }()

	nbSources := buildNBSnapshotSources(def.DBs)
	sbSources := buildSBSnapshotSources(def.DBs)
	snapshotSources := make([]history.TableSource, 0, len(nbSources)+len(sbSources))
	snapshotSources = append(snapshotSources, nbSources...)
	snapshotSources = append(snapshotSources, sbSources...)
	historyCollector := history.NewCollector(historyStore, def.EventHub, snapshotSources, cfg.SnapshotInterval, cfg.EventRetention)
	if cfg.EventMaxCount > 0 {
		historyCollector.SetEventMaxCount(cfg.EventMaxCount)
	}
	stopHistory := historyCollector.Start(context.Background())
	defer stopHistory()

	// Prometheus registry
	promRegistry := prometheus.NewRegistry()
	promRegistry.MustRegister(collectors.NewGoCollector())
	promRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsCollector := telemetry.NewCollector(def.DBs.NB, def.DBs.SB)
	promRegistry.MustRegister(metricsCollector)
	httpMetrics := telemetry.NewMiddleware(promRegistry)

	srv := api.NewServer(cfg.Listen, def.DBs, httpMetrics.Wrap)
	mux := srv.Mux()

	multiCluster := reg.Len() > 1

	// Write operations (optional, uses default cluster)
	if cfg.WriteEnabled {
		auditStore, err := write.NewAuditStore(ctx, historyStore.DB())
		if err != nil {
			return fmt.Errorf("creating audit store: %w", err)
		}
		impactResolver := impact.NewResolver(def.DBs.NB, def.DBs.SB)
		writeEngine, err := write.NewEngine(def.DBs.NB, def.DBs.SB, write.DefaultRegistry(), historyCollector, auditStore, cfg.WritePlanTTL, cfg.WriteRateLimit)
		if err != nil {
			return fmt.Errorf("creating write engine: %w", err)
		}
		writeEngine.SetResolver(impactResolver)
		stopWriteEngine := writeEngine.Start(context.Background())
		stopFuncs = append(stopFuncs, stopWriteEngine)
		handler.RegisterWrite(mux, writeEngine)
		handler.RegisterFailover(mux, writeEngine)
		handler.RegisterImpact(mux, impactResolver)
		fmt.Println("Write operations enabled")
	}

	// Trace store (shared)
	traceStore := handler.NewTraceStore(1 * time.Hour)

	wsOrigins := handler.ParseWSAllowedOrigins(cfg.WSAllowedOrigins)
	registerDefaultRoutes(mux, reg, def, cfg, historyStore, historyCollector, promRegistry, traceStore, wsOrigins, multiCluster, ovsPool, snapInfo)

	// The cluster proxy serves /api/v1/clusters/{name}/... for every cluster. It
	// is always registered — even with a single live cluster — so a snapshot
	// loaded at runtime becomes reachable as an additional cluster without a
	// restart.
	proxy := handler.RegisterClusterProxy(mux, reg, func(subMux *http.ServeMux, c *cluster.Cluster) {
		registerClusterRoutes(subMux, c, traceStore, wsOrigins, cfg.ChassisStaleThreshold)
	})
	if multiCluster {
		fmt.Printf("Multi-cluster mode enabled with %d clusters\n", reg.Len())
	}

	// Snapshot loading: a stored history snapshot can be loaded from the UI as a
	// read-only snapshot cluster, served by its own in-memory OVSDB servers.
	nbModel, err := nb.FullDatabaseModel()
	if err != nil {
		return fmt.Errorf("creating NB model: %w", err)
	}
	sbModel, err := sb.FullDatabaseModel()
	if err != nil {
		return fmt.Errorf("creating SB model: %w", err)
	}
	snapManager := snapshotsession.New(
		historyStore, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
			bctx, bcancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer bcancel()
			return buildSnapshotCluster(bctx, name, label, nbAddr, sbAddr)
		},
		func(c *cluster.Cluster) {
			subMux := http.NewServeMux()
			registerSnapshotClusterRoutes(subMux, c, traceStore, cfg.ChassisStaleThreshold)
			proxy.Add(c.Name, subMux)
		},
		proxy.Remove,
		def.DBs, // live OVN: suspended while a snapshot is loaded, resumed on eject
	)
	defer snapManager.Close()
	handler.RegisterSnapshotLoad(mux, snapManager)

	handler.RegisterAPICatchAll(mux)
	handler.RegisterUI(mux, northwatchUI.DistFS)

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(context.Background())
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		fmt.Printf("\nReceived %v, shutting down...\n", sig)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// setupSnapshotMode loads the snapshot file referenced by cfg.SnapshotFile,
// starts in-memory OVSDB servers backing it, and rewrites cfg.Clusters to a
// single cluster pointing at those local sockets. It returns the snapshot
// metadata (for the UI mode indicator) and a function that shuts the servers
// down, which must be called on exit.
func setupSnapshotMode(cfg *config.Config) (*handler.SnapshotInfo, func(), error) {
	fmt.Printf("Loading snapshot from %s...\n", cfg.SnapshotFile)
	// The snapshot path is an operator-provided CLI argument (like --config-file),
	// not untrusted input, so reading it directly is intentional.
	data, err := os.ReadFile(cfg.SnapshotFile) // #nosec G703
	if err != nil {
		return nil, nil, fmt.Errorf("opening snapshot: %w", err)
	}
	snap, err := snapshot.Load(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("reading snapshot: %w", err)
	}

	nbModel, err := nb.FullDatabaseModel()
	if err != nil {
		return nil, nil, fmt.Errorf("creating NB model: %w", err)
	}
	sbModel, err := sb.FullDatabaseModel()
	if err != nil {
		return nil, nil, fmt.Errorf("creating SB model: %w", err)
	}

	servers, err := snapshot.Serve(snap, nbModel, sbModel, nb.Schema(), sb.Schema())
	if err != nil {
		return nil, nil, fmt.Errorf("serving snapshot: %w", err)
	}

	cfg.Clusters = []config.ClusterConfig{{
		Name:      "default",
		Label:     "Snapshot",
		OVNNBAddr: servers.NBAddr,
		OVNSBAddr: servers.SBAddr,
	}}

	// Offline replay connects to local in-memory servers, so there is nothing to
	// protect: disable staged monitoring to keep the offline mode fast,
	// regardless of the --monitor-batch-delay default.
	cfg.MonitorBatchDelay = 0

	info := &handler.SnapshotInfo{CreatedAt: snap.CreatedAt}
	if snap.Source != nil {
		info.NBAddr = snap.Source.NBAddr
		info.SBAddr = snap.Source.SBAddr
	}

	fmt.Printf("Snapshot loaded (NB: %d rows, SB: %d rows); serving offline copy\n", snap.NB.RowCount(), snap.SB.RowCount())
	return info, servers.Close, nil
}

// buildCluster initializes all subsystems for a single OVN cluster and
// returns the populated *cluster.Cluster along with the cleanup functions
// that should run on shutdown.
func buildCluster(ctx context.Context, cfg *config.Config, cc config.ClusterConfig) (*cluster.Cluster, []func(), error) {
	fmt.Printf("Connecting to OVN databases for cluster %q...\n", cc.Name)

	nbModel, err := nb.FullDatabaseModel()
	if err != nil {
		return nil, nil, fmt.Errorf("cluster %q: creating NB model: %w", cc.Name, err)
	}
	sbModel, err := sb.FullDatabaseModel()
	if err != nil {
		return nil, nil, fmt.Errorf("cluster %q: creating SB model: %w", cc.Name, err)
	}

	mon := ovndb.MonitorOptions{
		BatchDelay: cfg.MonitorBatchDelay,
		SkipTables: cfg.MonitorSkipTables,
		// Startup --snapshot mode connects to in-memory servers with no "_Server"
		// database; skip the Raft monitors to avoid noisy connect warnings.
		SkipServerMonitors: cfg.SnapshotFile != "",
	}
	dbs, err := ovndb.Connect(ctx, cc.OVNNBAddr, cc.OVNSBAddr, nbModel, sbModel, mon)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster %q: connecting to OVN: %w", cc.Name, err)
	}
	fmt.Printf("Connected to OVN databases for cluster %q\n", cc.Name)

	enricher, err := buildEnricher(ctx, cfg, cc)
	if err != nil {
		dbs.Close()
		return nil, nil, fmt.Errorf("cluster %q: %w", cc.Name, err)
	}

	eventHub := events.NewHub()
	dbs.NB.Cache().AddEventHandler(events.NewBridge(eventHub, "nb"))
	dbs.SB.Cache().AddEventHandler(events.NewBridge(eventHub, "sb"))

	diagnoser := &debug.PortDiagnoser{NB: dbs.NB, SB: dbs.SB}
	connectivityChecker := &debug.ConnectivityChecker{NB: dbs.NB, SB: dbs.SB}
	aclAuditor := &debug.ACLAuditor{NB: dbs.NB}
	staleDetector := &debug.StaleDetector{NB: dbs.NB, SB: dbs.SB}

	flowDiffStore := flowdiff.NewStore(10000, 30*time.Minute)
	telemetryQuerier := telemetry.NewQuerier(dbs.NB, dbs.SB)
	// Per-endpoint "_Server" monitors feed real Raft cluster state (the member
	// list) into RaftHealth; the slices are empty in e.g. offline snapshot mode.
	telemetryQuerier.NBServers = dbs.NBServers
	telemetryQuerier.SBServers = dbs.SBServers
	propStore := telemetry.NewPropagationStore(50000, 24*time.Hour)
	propTracker := telemetry.NewPropagationTracker(eventHub, propStore, dbs.NB, dbs.SB)

	alertEngine := alert.NewEngine(eventHub, 30*time.Second)
	alertEngine.RegisterRule(alert.StaleChassis(dbs.NB, dbs.SB, 2))
	alertEngine.RegisterRule(alert.PortDown(dbs.SB))
	alertEngine.RegisterRule(alert.UnboundPort(dbs.SB))
	alertEngine.RegisterRule(alert.BFDDown(dbs.SB))
	alertEngine.RegisterRule(alert.FlowCountAnomaly(dbs.SB, 20.0))
	alertEngine.RegisterRule(alert.HAFailover(dbs.SB))

	if urls := alert.ParseWebhookURLs(cfg.AlertWebhookURLs); len(urls) > 0 {
		notifier := alert.NewWebhookNotifier(urls)
		alertEngine.SetNotifier(notifier.Notifier())
		fmt.Printf("Cluster %q: alert webhook notifications enabled (%d endpoints)\n", cc.Name, len(urls))
	}

	stopFuncs := []func(){
		flowdiff.StartCollector(eventHub, flowDiffStore),
		propTracker.Start(context.Background()),
		alertEngine.Start(context.Background()),
	}

	searchEngine := search.NewEngine([]search.DatabaseTables{
		{Name: "nb", Tables: buildNBSearchTables(dbs)},
		{Name: "sb", Tables: buildSBSearchTables(dbs)},
	})

	c := &cluster.Cluster{
		Name:                cc.Name,
		Label:               cc.Label,
		DBs:                 dbs,
		Correlator:          &correlate.Correlator{NB: dbs.NB, SB: dbs.SB},
		Enricher:            enricher,
		EventHub:            eventHub,
		SearchEngine:        searchEngine,
		FlowDiff:            flowDiffStore,
		AlertEngine:         alertEngine,
		Telemetry:           telemetryQuerier,
		ConnectivityChecker: connectivityChecker,
		PortDiagnoser:       diagnoser,
		ACLAuditor:          aclAuditor,
		StaleDetector:       staleDetector,
		PropagationStore:    propStore,
	}
	return c, stopFuncs, nil
}

// registerDefaultRoutes wires up all non-prefixed (single-cluster) routes on mux.
func registerDefaultRoutes(
	mux *http.ServeMux,
	reg *cluster.Registry,
	def *cluster.Cluster,
	cfg *config.Config,
	historyStore *history.Store,
	historyCollector *history.Collector,
	promRegistry *prometheus.Registry,
	traceStore *handler.TraceStore,
	wsOrigins []string,
	multiCluster bool,
	ovsPool *ovndb.OVSPool,
	snapInfo *handler.SnapshotInfo,
) {
	handler.RegisterHealth(mux, def.DBs)
	handler.RegisterCapabilities(mux, def.Enricher.HasProvider(), cfg.WriteEnabled, multiCluster, ovsPool != nil, snapInfo)
	if ovsPool != nil {
		handler.RegisterOVS(mux, ovsPool)
	}
	handler.RegisterNB(mux, def.DBs.NB)
	handler.RegisterSB(mux, def.DBs.SB)
	handler.RegisterInventory(mux, def.DBs.SB, cfg.ChassisStaleThreshold)
	handler.RegisterCorrelated(mux, def.Correlator, def.Enricher)
	handler.RegisterWS(mux, def.EventHub, wsOrigins)
	handler.RegisterTopology(mux, def.DBs.NB, def.DBs.SB)
	handler.RegisterNATTopology(mux, def.DBs.NB)
	handler.RegisterLBTopology(mux, def.DBs.NB, def.DBs.SB)
	handler.RegisterGatewayHealth(mux, def.DBs.NB, def.DBs.SB)
	handler.RegisterNextHopMAC(mux, def.DBs.NB, def.DBs.SB)
	handler.RegisterFlows(mux, def.DBs.SB)
	handler.RegisterDebug(mux, def.ConnectivityChecker, def.PortDiagnoser, def.ACLAuditor, def.StaleDetector)
	handler.RegisterTrace(mux, def.DBs.SB, traceStore)
	handler.RegisterExport(mux, def.DBs.NB, def.DBs.SB, traceStore)
	handler.RegisterFlowDiff(mux, def.FlowDiff)
	handler.RegisterHistory(mux, historyStore, historyCollector)
	handler.RegisterSearch(mux, def.SearchEngine)
	handler.RegisterTelemetry(mux, def.Telemetry, promRegistry, def.PropagationStore)
	handler.RegisterAlerts(mux, def.AlertEngine)
	handler.RegisterClusters(mux, reg)
	handler.RegisterOpenAPI(mux, openapi.BuildSpec())
}

// registerClusterRoutes wires up the per-cluster routes on a sub-mux used by
// the cluster proxy. Telemetry is registered without a Prometheus registry
// because metrics are only exposed at the top level.
func registerClusterRoutes(subMux *http.ServeMux, c *cluster.Cluster, traceStore *handler.TraceStore, wsOrigins []string, chassisStaleThreshold time.Duration) {
	handler.RegisterNB(subMux, c.DBs.NB)
	handler.RegisterSB(subMux, c.DBs.SB)
	handler.RegisterInventory(subMux, c.DBs.SB, chassisStaleThreshold)
	handler.RegisterCorrelated(subMux, c.Correlator, c.Enricher)
	handler.RegisterTopology(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterNATTopology(subMux, c.DBs.NB)
	handler.RegisterLBTopology(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterGatewayHealth(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterNextHopMAC(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterFlows(subMux, c.DBs.SB)
	handler.RegisterSearch(subMux, c.SearchEngine)
	handler.RegisterFlowDiff(subMux, c.FlowDiff)
	handler.RegisterAlerts(subMux, c.AlertEngine)
	handler.RegisterTelemetry(subMux, c.Telemetry, nil, c.PropagationStore)
	handler.RegisterWS(subMux, c.EventHub, wsOrigins)
	handler.RegisterDebug(subMux, c.ConnectivityChecker, c.PortDiagnoser, c.ACLAuditor, c.StaleDetector)
	handler.RegisterTrace(subMux, c.DBs.SB, traceStore)
	handler.RegisterExport(subMux, c.DBs.NB, c.DBs.SB, traceStore)
}

// buildSnapshotCluster connects read-only clients to the in-memory OVSDB servers
// backing a loaded snapshot and assembles a cluster with only the browsing
// subsystems. Live-tracking subsystems (alerts, flow diff, telemetry,
// propagation) are intentionally omitted — a snapshot is a static point in time,
// so there is nothing for them to track and they would only add background load.
func buildSnapshotCluster(ctx context.Context, name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
	nbModel, err := nb.FullDatabaseModel()
	if err != nil {
		return nil, nil, fmt.Errorf("creating NB model: %w", err)
	}
	sbModel, err := sb.FullDatabaseModel()
	if err != nil {
		return nil, nil, fmt.Errorf("creating SB model: %w", err)
	}

	// The servers are local in-memory copies: load everything in one request and
	// skip the "_Server" Raft monitors (the snapshot exposes no "_Server" DB).
	dbs, err := ovndb.Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, ovndb.MonitorOptions{SkipServerMonitors: true})
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to snapshot servers: %w", err)
	}

	c := &cluster.Cluster{
		Name:       name,
		Label:      label,
		Mode:       "snapshot",
		DBs:        dbs,
		Correlator: &correlate.Correlator{NB: dbs.NB, SB: dbs.SB},
		Enricher:   enrich.NewEnricher(nil, 0),
		SearchEngine: search.NewEngine([]search.DatabaseTables{
			{Name: "nb", Tables: buildNBSearchTables(dbs)},
			{Name: "sb", Tables: buildSBSearchTables(dbs)},
		}),
		ConnectivityChecker: &debug.ConnectivityChecker{NB: dbs.NB, SB: dbs.SB},
		PortDiagnoser:       &debug.PortDiagnoser{NB: dbs.NB, SB: dbs.SB},
		ACLAuditor:          &debug.ACLAuditor{NB: dbs.NB},
		StaleDetector:       &debug.StaleDetector{NB: dbs.NB, SB: dbs.SB},
	}
	return c, nil, nil
}

// registerSnapshotClusterRoutes wires the read-only browsing routes for a loaded
// snapshot cluster. Live-only routes (alerts, telemetry, flow diff, websocket)
// are intentionally omitted, matching the subsystems buildSnapshotCluster builds.
func registerSnapshotClusterRoutes(subMux *http.ServeMux, c *cluster.Cluster, traceStore *handler.TraceStore, chassisStaleThreshold time.Duration) {
	handler.RegisterNB(subMux, c.DBs.NB)
	handler.RegisterSB(subMux, c.DBs.SB)
	handler.RegisterInventory(subMux, c.DBs.SB, chassisStaleThreshold)
	handler.RegisterCorrelated(subMux, c.Correlator, c.Enricher)
	handler.RegisterTopology(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterNATTopology(subMux, c.DBs.NB)
	handler.RegisterLBTopology(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterGatewayHealth(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterNextHopMAC(subMux, c.DBs.NB, c.DBs.SB)
	handler.RegisterFlows(subMux, c.DBs.SB)
	handler.RegisterSearch(subMux, c.SearchEngine)
	handler.RegisterDebug(subMux, c.ConnectivityChecker, c.PortDiagnoser, c.ACLAuditor, c.StaleDetector)
	handler.RegisterTrace(subMux, c.DBs.SB, traceStore)
	handler.RegisterExport(subMux, c.DBs.NB, c.DBs.SB, traceStore)
}

// buildEnricher creates an enricher for a cluster based on its config.
func buildEnricher(ctx context.Context, cfg *config.Config, cc config.ClusterConfig) (*enrich.Enricher, error) {
	if cc.Enrichment != nil {
		switch cc.Enrichment.Type {
		case "kubernetes":
			fmt.Printf("Cluster %q: setting up Kubernetes enrichment...\n", cc.Name)
			provider, err := enrich.NewKubernetesProvider(ctx, cc.Enrichment.Kubeconfig, cc.Enrichment.KubeContext)
			if err != nil {
				return nil, fmt.Errorf("creating Kubernetes provider: %w", err)
			}
			fmt.Printf("Cluster %q: Kubernetes enrichment enabled\n", cc.Name)
			return enrich.NewEnricher(provider, cfg.EnrichmentCacheTTL), nil

		case "openstack":
			fmt.Printf("Cluster %q: authenticating with OpenStack...\n", cc.Name)
			// Build a temporary config-like struct for the OpenStack provider
			osCfg := &config.Config{
				OpenStackAuthURL:     cc.Enrichment.OpenStackAuthURL,
				OpenStackUsername:    cc.Enrichment.OpenStackUsername,
				OpenStackPassword:    cc.Enrichment.OpenStackPassword,
				OpenStackProjectName: cc.Enrichment.OpenStackProjectName,
				OpenStackDomainName:  cc.Enrichment.OpenStackDomainName,
				OpenStackRegionName:  cc.Enrichment.OpenStackRegionName,
				OpenStackCACert:      cc.Enrichment.OpenStackCACert,
			}
			provider, err := enrich.NewOpenStackProvider(ctx, osCfg)
			if err != nil {
				return nil, fmt.Errorf("creating OpenStack provider: %w", err)
			}
			fmt.Printf("Cluster %q: OpenStack enrichment enabled\n", cc.Name)
			return enrich.NewEnricher(provider, cfg.EnrichmentCacheTTL), nil
		}
	}

	// Fallback: check legacy flat flags for the default cluster
	if cc.Name == "default" {
		if cfg.KubeEnrichment {
			fmt.Println("Setting up Kubernetes enrichment...")
			provider, err := enrich.NewKubernetesProvider(ctx, cfg.Kubeconfig, cfg.KubeContext)
			if err != nil {
				return nil, fmt.Errorf("creating Kubernetes provider: %w", err)
			}
			fmt.Println("Kubernetes enrichment enabled")
			return enrich.NewEnricher(provider, cfg.EnrichmentCacheTTL), nil
		}
		if cfg.OpenStackAuthURL != "" {
			fmt.Println("Authenticating with OpenStack...")
			provider, err := enrich.NewOpenStackProvider(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("creating OpenStack provider: %w", err)
			}
			fmt.Println("OpenStack enrichment enabled")
			return enrich.NewEnricher(provider, cfg.EnrichmentCacheTTL), nil
		}
	}

	return enrich.NewEnricher(nil, 0), nil
}

func buildNBSearchTables(dbs *ovndb.OVNDatabases) []search.TableDef {
	c := dbs.NB
	return []search.TableDef{
		search.RegisterTable[nb.LogicalSwitch]("Logical_Switch", c),
		search.RegisterTable[nb.LogicalSwitchPort]("Logical_Switch_Port", c),
		search.RegisterTable[nb.LogicalRouter]("Logical_Router", c),
		search.RegisterTable[nb.LogicalRouterPort]("Logical_Router_Port", c),
		search.RegisterTable[nb.ACL]("ACL", c),
		search.RegisterTable[nb.NAT]("NAT", c),
		search.RegisterTable[nb.AddressSet]("Address_Set", c),
		search.RegisterTable[nb.PortGroup]("Port_Group", c),
		search.RegisterTable[nb.LoadBalancer]("Load_Balancer", c),
		search.RegisterTable[nb.DHCPOptions]("DHCP_Options", c),
		search.RegisterTable[nb.LogicalRouterStaticRoute]("Logical_Router_Static_Route", c),
		search.RegisterTable[nb.LogicalRouterPolicy]("Logical_Router_Policy", c),
		search.RegisterTable[nb.DNS]("DNS", c),
		search.RegisterTable[nb.StaticMACBinding]("Static_MAC_Binding", c),
	}
}

func buildSBSearchTables(dbs *ovndb.OVNDatabases) []search.TableDef {
	c := dbs.SB
	return []search.TableDef{
		search.RegisterTable[sb.Chassis]("Chassis", c),
		search.RegisterTable[sb.PortBinding]("Port_Binding", c),
		search.RegisterTable[sb.LogicalFlow]("Logical_Flow", c),
		search.RegisterTable[sb.DatapathBinding]("Datapath_Binding", c),
		search.RegisterTable[sb.Encap]("Encap", c),
		search.RegisterTable[sb.MACBinding]("MAC_Binding", c),
		search.RegisterTable[sb.FDB]("FDB", c),
		search.RegisterTable[sb.AddressSet]("Address_Set", c),
		search.RegisterTable[sb.DNS]("DNS", c),
		search.RegisterTable[sb.LoadBalancer]("Load_Balancer", c),
		search.RegisterTable[sb.StaticMACBinding]("Static_MAC_Binding", c),
	}
}

func snapshotSource[T any](database, table string, c client.Client) history.TableSource {
	return history.TableSource{
		Database: database,
		Table:    table,
		ListFunc: func(ctx context.Context) ([]map[string]any, error) {
			var results []T
			if err := c.List(ctx, &results); err != nil {
				return nil, err
			}
			return api.ModelsToMaps(results), nil
		},
	}
}

func buildNBSnapshotSources(dbs *ovndb.OVNDatabases) []history.TableSource {
	c := dbs.NB
	return []history.TableSource{
		snapshotSource[nb.LogicalSwitch]("nb", "Logical_Switch", c),
		snapshotSource[nb.LogicalSwitchPort]("nb", "Logical_Switch_Port", c),
		snapshotSource[nb.LogicalRouter]("nb", "Logical_Router", c),
		snapshotSource[nb.LogicalRouterPort]("nb", "Logical_Router_Port", c),
		snapshotSource[nb.ACL]("nb", "ACL", c),
		snapshotSource[nb.NAT]("nb", "NAT", c),
		snapshotSource[nb.AddressSet]("nb", "Address_Set", c),
		snapshotSource[nb.PortGroup]("nb", "Port_Group", c),
		snapshotSource[nb.LoadBalancer]("nb", "Load_Balancer", c),
		snapshotSource[nb.DHCPOptions]("nb", "DHCP_Options", c),
		snapshotSource[nb.LogicalRouterStaticRoute]("nb", "Logical_Router_Static_Route", c),
		snapshotSource[nb.LogicalRouterPolicy]("nb", "Logical_Router_Policy", c),
		snapshotSource[nb.DNS]("nb", "DNS", c),
		snapshotSource[nb.StaticMACBinding]("nb", "Static_MAC_Binding", c),
	}
}

func buildSBSnapshotSources(dbs *ovndb.OVNDatabases) []history.TableSource {
	c := dbs.SB
	return []history.TableSource{
		snapshotSource[sb.Chassis]("sb", "Chassis", c),
		snapshotSource[sb.PortBinding]("sb", "Port_Binding", c),
		snapshotSource[sb.LogicalFlow]("sb", "Logical_Flow", c),
		snapshotSource[sb.DatapathBinding]("sb", "Datapath_Binding", c),
		snapshotSource[sb.Encap]("sb", "Encap", c),
		snapshotSource[sb.MACBinding]("sb", "MAC_Binding", c),
		snapshotSource[sb.FDB]("sb", "FDB", c),
		snapshotSource[sb.AddressSet]("sb", "Address_Set", c),
		snapshotSource[sb.DNS]("sb", "DNS", c),
		snapshotSource[sb.LoadBalancer]("sb", "Load_Balancer", c),
		snapshotSource[sb.StaticMACBinding]("sb", "Static_MAC_Binding", c),
	}
}
