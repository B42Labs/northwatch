package ovsdb

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb/serverdb"
)

// ServerMonitor is a best-effort connection to the special "_Server" database
// on a single OVSDB endpoint. Its Database table exposes that one server's view
// of Raft cluster state (its server ID, whether it is the leader, its log index,
// and whether it is in contact with the cluster). One ServerMonitor is created
// per configured endpoint so the members can be aggregated into a cluster view.
type ServerMonitor struct {
	// Endpoint is the individual address this monitor targets (e.g.
	// "tcp:10.0.0.1:6641"). It is always set, even when Client is nil, so an
	// unreachable member is still listed.
	Endpoint string
	// Client is the connected "_Server" monitor, or nil when the endpoint could
	// not be reached or does not expose "_Server" (standalone server, offline
	// snapshot, or down at startup).
	Client client.Client
}

// OVNDatabases bundles the Northbound and Southbound libovsdb client
// connections used by Northwatch's handlers and subsystems.
type OVNDatabases struct {
	NB client.Client
	SB client.Client

	// NBServers and SBServers hold one best-effort "_Server" monitor per
	// configured NB/SB endpoint. Each server's "_Server" Database table reports
	// only its own Raft view, so aggregating across endpoints reconstructs the
	// member list. The slices are empty when no endpoint exposes "_Server"
	// (standalone ovsdb-server, offline snapshot replay), in which case the Raft
	// health view degrades to "unavailable" rather than failing.
	NBServers []ServerMonitor
	SBServers []ServerMonitor
}

// MonitorOptions tunes how the initial libovsdb monitor is built when
// populating the cache. The zero value reproduces the original behavior: a
// single MonitorAll request covering every table and column at once.
//
// In very large OVN deployments that one atomic request forces the (often
// single-threaded) ovsdb-server to serialize the entire database into one
// reply, which is the main source of load when Northwatch starts or captures a
// snapshot. These options let an operator spread that work out and/or skip the
// largest tables.
type MonitorOptions struct {
	// BatchDelay, when > 0, switches from one MonitorAll request to staged
	// per-table monitor requests, sleeping BatchDelay between each. Each table's
	// initial dump then arrives as its own smaller reply instead of one giant
	// one, bounding peak memory on both the server and Northwatch — at the cost
	// of a slower startup. On a libovsdb reconnect the monitors are re-issued
	// without the delay, but they remain separate (small) replies.
	BatchDelay time.Duration

	// SkipTables lists table names that are never monitored. Use it to exclude
	// the largest Southbound tables (e.g. Logical_Flow, MAC_Binding, FDB) in
	// huge deployments. Features that read a skipped table will see it as empty.
	SkipTables []string
}

func newBackoff() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 0 // retry forever
	return bo
}

// endpointList splits a comma-separated OVSDB address into individual,
// trimmed endpoint strings (e.g. "tcp:10.0.0.1:6641,tcp:10.0.0.2:6641" →
// ["tcp:10.0.0.1:6641", "tcp:10.0.0.2:6641"]).
func endpointList(addr string) []string {
	parts := strings.Split(addr, ",")
	eps := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			eps = append(eps, p)
		}
	}
	return eps
}

// splitEndpoints turns a comma-separated list of OVSDB addresses into
// individual WithEndpoint options. This enables libovsdb's native failover
// when multiple endpoints are provided.
func splitEndpoints(addr string) []client.Option {
	eps := endpointList(addr)
	opts := make([]client.Option, 0, len(eps))
	for _, ep := range eps {
		opts = append(opts, client.WithEndpoint(ep))
	}
	return opts
}

// Connect dials both the Northbound and Southbound OVSDB servers, populates
// the libovsdb monitor cache, and returns a ready-to-use OVNDatabases handle.
// The mon options control how the initial cache is built; pass the zero value
// for the default single-request MonitorAll behavior. On any failure, it closes
// the partially-opened clients before returning.
func Connect(ctx context.Context, nbAddr, sbAddr string, nbModel, sbModel model.ClientDBModel, mon MonitorOptions) (*OVNDatabases, error) {
	// Create clients sequentially to avoid race in libovsdb's stdr.SetVerbosity.
	// Each client gets its own backoff instance since ExponentialBackOff is stateful.
	nbOpts := append(splitEndpoints(nbAddr), client.WithReconnect(10*time.Second, newBackoff()))
	nbClient, err := client.NewOVSDBClient(nbModel, nbOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating NB client: %w", err)
	}

	sbOpts := append(splitEndpoints(sbAddr), client.WithReconnect(10*time.Second, newBackoff()))
	sbClient, err := client.NewOVSDBClient(sbModel, sbOpts...)
	if err != nil {
		nbClient.Close()
		return nil, fmt.Errorf("creating SB client: %w", err)
	}

	// Connect and monitor concurrently
	var (
		nbErr, sbErr error
		wg           sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		nbErr = connectAndMonitor(ctx, nbClient, nbAddr, nbModel, mon)
	}()
	go func() {
		defer wg.Done()
		sbErr = connectAndMonitor(ctx, sbClient, sbAddr, sbModel, mon)
	}()
	wg.Wait()

	if nbErr != nil {
		sbClient.Close()
		return nil, fmt.Errorf("connecting to NB: %w", nbErr)
	}
	if sbErr != nil {
		nbClient.Close()
		return nil, fmt.Errorf("connecting to SB: %w", sbErr)
	}

	dbs := &OVNDatabases{NB: nbClient, SB: sbClient}

	// Best-effort: monitor the special "_Server" database on every configured
	// endpoint so the Raft health view can report the full member list. Done
	// after the primary clients connect and never fatal — see
	// connectServerMonitors.
	dbs.NBServers = connectServerMonitors(ctx, nbAddr)
	dbs.SBServers = connectServerMonitors(ctx, sbAddr)

	return dbs, nil
}

// connectServerMonitors opens one best-effort "_Server" monitor per endpoint in
// addr. Each server's "_Server" Database table reports only its own Raft view,
// so a separate connection per endpoint is needed to enumerate cluster members.
//
// It returns one ServerMonitor per endpoint (Client nil when that endpoint is
// unreachable or does not expose "_Server"). This is intentionally non-fatal:
// standalone servers and offline snapshots simply yield monitors with nil
// Clients and the Raft view reports "unavailable" instead of failing startup.
//
// Clients are created sequentially (matching the note in Connect about
// libovsdb's global logger) but connected concurrently with a per-endpoint
// timeout, so a slow or down endpoint does not serialize startup. Once
// connected, WithReconnect keeps each monitor healthy across drops.
func connectServerMonitors(ctx context.Context, addr string) []ServerMonitor {
	eps := endpointList(addr)
	if len(eps) == 0 {
		return nil
	}

	sm, err := serverdb.FullDatabaseModel()
	if err != nil {
		fmt.Printf("warning: building _Server model (Raft health unavailable): %v\n", err)
		return nil
	}

	monitors := make([]ServerMonitor, len(eps))
	clients := make([]client.Client, len(eps))
	for i, ep := range eps {
		monitors[i].Endpoint = ep
		c, err := client.NewOVSDBClient(sm, client.WithEndpoint(ep), client.WithReconnect(10*time.Second, newBackoff()))
		if err != nil {
			fmt.Printf("warning: creating _Server client for %s: %v\n", ep, err)
			continue
		}
		clients[i] = c
	}

	var wg sync.WaitGroup
	for i := range clients {
		if clients[i] == nil {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := clients[i]
			connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := c.Connect(connCtx); err != nil {
				fmt.Printf("warning: connecting _Server on %s (member unreachable): %v\n", monitors[i].Endpoint, err)
				c.Close()
				return
			}
			if _, err := c.MonitorAll(connCtx); err != nil {
				fmt.Printf("warning: monitoring _Server on %s (member unreachable): %v\n", monitors[i].Endpoint, err)
				c.Close()
				return
			}
			monitors[i].Client = c
		}(i)
	}
	wg.Wait()

	return monitors
}

func connectAndMonitor(ctx context.Context, c client.Client, addr string, dbModel model.ClientDBModel, mon MonitorOptions) error {
	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to %s: %w", addr, err)
	}

	if err := startMonitor(ctx, c, dbModel, mon); err != nil {
		c.Close()
		return fmt.Errorf("monitoring %s: %w", addr, err)
	}

	return nil
}

// startMonitor populates the client cache. With the default (zero) options it
// issues a single MonitorAll — every table and column in one request, exactly
// as before. When MonitorOptions asks to skip tables or to batch, it builds
// explicit per-table monitors instead so the initial dump is split into smaller
// replies and, optionally, spread over time.
func startMonitor(ctx context.Context, c client.Client, dbModel model.ClientDBModel, mon MonitorOptions) error {
	// Fast path: unchanged behavior when nothing is customized.
	if mon.BatchDelay <= 0 && len(mon.SkipTables) == 0 {
		_, err := c.MonitorAll(ctx)
		return err
	}

	tables := monitoredTables(dbModel, mon.SkipTables)
	if len(tables) == 0 {
		return fmt.Errorf("no tables left to monitor after applying skip list")
	}

	// Skip list but no batching: a single monitor over the remaining tables,
	// matching MonitorAll's one-request semantics minus the excluded tables.
	if mon.BatchDelay <= 0 {
		m := c.NewMonitor()
		for _, t := range tables {
			m.Tables = append(m.Tables, client.TableMonitor{Table: t})
		}
		_, err := c.Monitor(ctx, m)
		return err
	}

	// Staged: one monitor request per table, sleeping between them so the server
	// serializes and ships each table's initial dump separately over time. Since
	// a staged load is deliberately slow, log each table's progress and timing to
	// stdout so an operator can see what is loading and spot a slow table. NB and
	// SB stage concurrently and their lines interleave, so prefix each with the
	// database name.
	dbName := dbModel.Name()
	stagedStart := time.Now()
	for i, t := range tables {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(mon.BatchDelay):
			}
		}
		fmt.Printf("[%s] loading table %s (%d/%d)...\n", dbName, t, i+1, len(tables))
		m := c.NewMonitor()
		m.Tables = append(m.Tables, client.TableMonitor{Table: t})
		start := time.Now()
		if _, err := c.Monitor(ctx, m); err != nil {
			return fmt.Errorf("monitoring table %s: %w", t, err)
		}
		fmt.Printf("[%s] loaded table %s (%d/%d) in %s\n", dbName, t, i+1, len(tables), time.Since(start).Round(time.Millisecond))
	}
	fmt.Printf("[%s] staged load complete: %d tables in %s\n", dbName, len(tables), time.Since(stagedStart).Round(time.Millisecond))
	return nil
}

// monitoredTables returns the sorted table names of a database model, excluding
// any whose name appears in skip. Sorting keeps the monitor order deterministic.
func monitoredTables(dbModel model.ClientDBModel, skip []string) []string {
	skipSet := make(map[string]struct{}, len(skip))
	for _, s := range skip {
		if s = strings.TrimSpace(s); s != "" {
			skipSet[s] = struct{}{}
		}
	}

	types := dbModel.Types()
	tables := make([]string, 0, len(types))
	for name := range types {
		if _, skipped := skipSet[name]; skipped {
			continue
		}
		tables = append(tables, name)
	}
	sort.Strings(tables)
	return tables
}

// Ready reports whether both NB and SB clients currently have an active
// connection to their OVSDB servers.
func (d *OVNDatabases) Ready() bool {
	return d.NB.Connected() && d.SB.Connected()
}

// Close shuts down the NB and SB OVSDB clients along with every best-effort
// "_Server" member monitor that connected.
func (d *OVNDatabases) Close() {
	d.NB.Close()
	d.SB.Close()
	for _, m := range d.NBServers {
		if m.Client != nil {
			m.Client.Close()
		}
	}
	for _, m := range d.SBServers {
		if m.Client != nil {
			m.Client.Close()
		}
	}
}
