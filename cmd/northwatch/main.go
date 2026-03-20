package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/api/handler"
	"github.com/b42labs/northwatch/internal/config"
	"github.com/b42labs/northwatch/internal/correlate"
	"github.com/b42labs/northwatch/internal/debug"
	"github.com/b42labs/northwatch/internal/enrich"
	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/flowdiff"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/search"
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

	srv := api.NewServer(cfg.Listen, dbs)
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

	searchEngine := search.NewEngine(
		buildNBSearchTables(dbs),
		buildSBSearchTables(dbs),
	)
	handler.RegisterSearch(mux, searchEngine)
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
