package main

import (
	"context"
	"fmt"
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
	"github.com/b42labs/northwatch/internal/config"
	"github.com/b42labs/northwatch/internal/correlate"
	"github.com/b42labs/northwatch/internal/debug"
	"github.com/b42labs/northwatch/internal/enrich"
	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/flowdiff"
	"github.com/b42labs/northwatch/internal/history"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/search"
	"github.com/b42labs/northwatch/internal/telemetry"
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

	fmt.Println("Connecting to OVN databases...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nbModel, err := nb.FullDatabaseModel()
	if err != nil {
		return fmt.Errorf("creating NB model: %w", err)
	}
	sbModel, err := sb.FullDatabaseModel()
	if err != nil {
		return fmt.Errorf("creating SB model: %w", err)
	}

	dbs, err := ovndb.Connect(ctx, cfg.OVNNBAddr, cfg.OVNSBAddr, nbModel, sbModel)
	if err != nil {
		return fmt.Errorf("connecting to OVN: %w", err)
	}
	defer dbs.Close()
	fmt.Println("Connected to OVN databases")

	// Correlation engine
	cor := &correlate.Correlator{NB: dbs.NB, SB: dbs.SB}

	// Enrichment provider (optional)
	var enricher *enrich.Enricher
	if cfg.OpenStackAuthURL != "" {
		fmt.Println("Authenticating with OpenStack...")
		provider, provErr := enrich.NewOpenStackProvider(ctx, cfg)
		if provErr != nil {
			return fmt.Errorf("creating OpenStack provider: %w", provErr)
		}
		enricher = enrich.NewEnricher(provider, cfg.EnrichmentCacheTTL)
		fmt.Println("OpenStack enrichment enabled")
	} else {
		enricher = enrich.NewEnricher(nil, 0)
	}

	// Real-time event hub
	eventHub := events.NewHub()
	dbs.NB.Cache().AddEventHandler(events.NewBridge(eventHub, "nb"))
	dbs.SB.Cache().AddEventHandler(events.NewBridge(eventHub, "sb"))

	// Debug tools
	diagnoser := &debug.PortDiagnoser{NB: dbs.NB, SB: dbs.SB}
	connectivityChecker := &debug.ConnectivityChecker{NB: dbs.NB, SB: dbs.SB}

	// Flow diff tracking
	flowDiffStore := flowdiff.NewStore(10000, 30*time.Minute)
	stopCollector := flowdiff.StartCollector(eventHub, flowDiffStore)
	defer stopCollector()

	// History & snapshot store
	historyStore, err := history.NewStore(cfg.HistoryDBPath)
	if err != nil {
		return fmt.Errorf("opening history database: %w", err)
	}
	defer func() { _ = historyStore.Close() }()

	snapshotSources := append(buildNBSnapshotSources(dbs), buildSBSnapshotSources(dbs)...)
	historyCollector := history.NewCollector(historyStore, eventHub, snapshotSources, cfg.SnapshotInterval, cfg.EventRetention)
	if cfg.EventMaxCount > 0 {
		historyCollector.SetEventMaxCount(cfg.EventMaxCount)
	}
	stopHistory := historyCollector.Start(context.Background())
	defer stopHistory()

	// Prometheus registry
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsCollector := telemetry.NewCollector(dbs.NB, dbs.SB)
	registry.MustRegister(metricsCollector)
	httpMetrics := telemetry.NewMiddleware(registry)

	// Telemetry querier
	telemetryQuerier := telemetry.NewQuerier(dbs.NB, dbs.SB)

	// Alert engine
	alertEngine := alert.NewEngine(eventHub, 30*time.Second)
	alertEngine.RegisterRule(alert.StaleChassis(dbs.NB, dbs.SB, 2))
	alertEngine.RegisterRule(alert.PortDown(dbs.SB))
	alertEngine.RegisterRule(alert.UnboundPort(dbs.SB))
	alertEngine.RegisterRule(alert.BFDDown(dbs.SB))
	alertEngine.RegisterRule(alert.FlowCountAnomaly(dbs.SB, 20.0))

	// Webhook notifications (optional)
	if urls := alert.ParseWebhookURLs(cfg.AlertWebhookURLs); len(urls) > 0 {
		notifier := alert.NewWebhookNotifier(urls)
		alertEngine.SetNotifier(notifier.Notifier())
		fmt.Printf("Alert webhook notifications enabled (%d endpoints)\n", len(urls))
	}

	stopAlerts := alertEngine.Start(context.Background())
	defer stopAlerts()

	srv := api.NewServer(cfg.Listen, dbs, httpMetrics.Wrap)
	mux := srv.Mux()

	handler.RegisterHealth(mux, dbs)
	handler.RegisterCapabilities(mux, enricher.HasProvider())
	handler.RegisterNB(mux, dbs.NB)
	handler.RegisterSB(mux, dbs.SB)
	handler.RegisterCorrelated(mux, cor, enricher)
	handler.RegisterWS(mux, eventHub)
	handler.RegisterTopology(mux, dbs.NB, dbs.SB)
	handler.RegisterFlows(mux, dbs.SB)
	handler.RegisterDebug(mux, connectivityChecker, diagnoser)
	handler.RegisterTrace(mux, dbs.SB)
	handler.RegisterFlowDiff(mux, flowDiffStore)
	handler.RegisterHistory(mux, historyStore, historyCollector)

	searchEngine := search.NewEngine(
		buildNBSearchTables(dbs),
		buildSBSearchTables(dbs),
	)
	handler.RegisterSearch(mux, searchEngine)
	handler.RegisterTelemetry(mux, telemetryQuerier, registry)
	handler.RegisterAlerts(mux, alertEngine)
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
