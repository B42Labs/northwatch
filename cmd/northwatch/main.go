package main

import (
	"context"
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
	"github.com/b42labs/northwatch/internal/search"
	"github.com/b42labs/northwatch/internal/telemetry"
	"github.com/b42labs/northwatch/internal/write"
	northwatchUI "github.com/b42labs/northwatch/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build cluster registry from config
	reg := cluster.NewRegistry()
	var stopFuncs []func()

	for _, cc := range cfg.Clusters {
		fmt.Printf("Connecting to OVN databases for cluster %q...\n", cc.Name)

		nbModel, err := nb.FullDatabaseModel()
		if err != nil {
			return fmt.Errorf("cluster %q: creating NB model: %w", cc.Name, err)
		}
		sbModel, err := sb.FullDatabaseModel()
		if err != nil {
			return fmt.Errorf("cluster %q: creating SB model: %w", cc.Name, err)
		}

		dbs, err := ovndb.Connect(ctx, cc.OVNNBAddr, cc.OVNSBAddr, nbModel, sbModel)
		if err != nil {
			reg.Close() // close any already-connected clusters
			return fmt.Errorf("cluster %q: connecting to OVN: %w", cc.Name, err)
		}
		fmt.Printf("Connected to OVN databases for cluster %q\n", cc.Name)

		// Correlation engine
		cor := &correlate.Correlator{NB: dbs.NB, SB: dbs.SB}

		// Enrichment provider (optional)
		enricher, err := buildEnricher(ctx, cfg, cc)
		if err != nil {
			dbs.Close()
			reg.Close()
			return fmt.Errorf("cluster %q: %w", cc.Name, err)
		}

		// Real-time event hub
		eventHub := events.NewHub()
		dbs.NB.Cache().AddEventHandler(events.NewBridge(eventHub, "nb"))
		dbs.SB.Cache().AddEventHandler(events.NewBridge(eventHub, "sb"))

		// Debug tools
		diagnoser := &debug.PortDiagnoser{NB: dbs.NB, SB: dbs.SB}
		connectivityChecker := &debug.ConnectivityChecker{NB: dbs.NB, SB: dbs.SB}
		aclAuditor := &debug.ACLAuditor{NB: dbs.NB}
		staleDetector := &debug.StaleDetector{NB: dbs.NB, SB: dbs.SB}

		// Flow diff tracking
		flowDiffStore := flowdiff.NewStore(10000, 30*time.Minute)
		stopCollector := flowdiff.StartCollector(eventHub, flowDiffStore)
		stopFuncs = append(stopFuncs, stopCollector)

		// Telemetry querier
		telemetryQuerier := telemetry.NewQuerier(dbs.NB, dbs.SB)

		// Propagation tracker
		propStore := telemetry.NewPropagationStore(50000, 24*time.Hour)
		propTracker := telemetry.NewPropagationTracker(eventHub, propStore, dbs.NB, dbs.SB)
		stopPropTracker := propTracker.Start(context.Background())
		stopFuncs = append(stopFuncs, stopPropTracker)

		// Alert engine
		alertEngine := alert.NewEngine(eventHub, 30*time.Second)
		alertEngine.RegisterRule(alert.StaleChassis(dbs.NB, dbs.SB, 2))
		alertEngine.RegisterRule(alert.PortDown(dbs.SB))
		alertEngine.RegisterRule(alert.UnboundPort(dbs.SB))
		alertEngine.RegisterRule(alert.BFDDown(dbs.SB))
		alertEngine.RegisterRule(alert.FlowCountAnomaly(dbs.SB, 20.0))
		alertEngine.RegisterRule(alert.HAFailover(dbs.SB))

		// Webhook notifications (optional)
		if urls := alert.ParseWebhookURLs(cfg.AlertWebhookURLs); len(urls) > 0 {
			notifier := alert.NewWebhookNotifier(urls)
			alertEngine.SetNotifier(notifier.Notifier())
			fmt.Printf("Cluster %q: alert webhook notifications enabled (%d endpoints)\n", cc.Name, len(urls))
		}

		stopAlerts := alertEngine.Start(context.Background())
		stopFuncs = append(stopFuncs, stopAlerts)

		// Search engine
		searchEngine := search.NewEngine(
			buildNBSearchTables(dbs),
			buildSBSearchTables(dbs),
		)

		c := &cluster.Cluster{
			Name:                cc.Name,
			Label:               cc.Label,
			DBs:                 dbs,
			Correlator:          cor,
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
		reg.Register(cc.Name, c)
	}
	defer reg.Close()
	defer func() {
		for _, stop := range stopFuncs {
			stop()
		}
	}()

	def := reg.Default()

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

	// Register default (non-prefixed) routes using the default cluster
	handler.RegisterHealth(mux, def.DBs)
	handler.RegisterCapabilities(mux, def.Enricher.HasProvider(), cfg.WriteEnabled, multiCluster)
	handler.RegisterNB(mux, def.DBs.NB)
	handler.RegisterSB(mux, def.DBs.SB)
	handler.RegisterCorrelated(mux, def.Correlator, def.Enricher)
	wsOrigins := handler.ParseWSAllowedOrigins(cfg.WSAllowedOrigins)
	handler.RegisterWS(mux, def.EventHub, wsOrigins)
	handler.RegisterTopology(mux, def.DBs.NB, def.DBs.SB)
	handler.RegisterNATTopology(mux, def.DBs.NB)
	handler.RegisterLBTopology(mux, def.DBs.NB, def.DBs.SB)
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

	// Multi-cluster: register cluster-prefixed routes
	if multiCluster {
		handler.RegisterClusterProxy(mux, reg, func(subMux *http.ServeMux, c *cluster.Cluster) {
			handler.RegisterNB(subMux, c.DBs.NB)
			handler.RegisterSB(subMux, c.DBs.SB)
			handler.RegisterCorrelated(subMux, c.Correlator, c.Enricher)
			handler.RegisterTopology(subMux, c.DBs.NB, c.DBs.SB)
			handler.RegisterNATTopology(subMux, c.DBs.NB)
			handler.RegisterLBTopology(subMux, c.DBs.NB, c.DBs.SB)
			handler.RegisterFlows(subMux, c.DBs.SB)
			handler.RegisterSearch(subMux, c.SearchEngine)
			handler.RegisterFlowDiff(subMux, c.FlowDiff)
			handler.RegisterAlerts(subMux, c.AlertEngine)
			handler.RegisterTelemetry(subMux, c.Telemetry, nil, c.PropagationStore)
			handler.RegisterWS(subMux, c.EventHub, wsOrigins)
			handler.RegisterDebug(subMux, c.ConnectivityChecker, c.PortDiagnoser, c.ACLAuditor, c.StaleDetector)
			handler.RegisterTrace(subMux, c.DBs.SB, traceStore)
			handler.RegisterExport(subMux, c.DBs.NB, c.DBs.SB, traceStore)
		})
		fmt.Printf("Multi-cluster mode enabled with %d clusters\n", reg.Len())
	}

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
				OpenStackUsername:     cc.Enrichment.OpenStackUsername,
				OpenStackPassword:    cc.Enrichment.OpenStackPassword,
				OpenStackProjectName: cc.Enrichment.OpenStackProjectName,
				OpenStackDomainName:  cc.Enrichment.OpenStackDomainName,
				OpenStackRegionName:  cc.Enrichment.OpenStackRegionName,
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
