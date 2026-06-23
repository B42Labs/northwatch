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
)

// OVNDatabases bundles the Northbound and Southbound libovsdb client
// connections used by Northwatch's handlers and subsystems.
type OVNDatabases struct {
	NB client.Client
	SB client.Client
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

// splitEndpoints parses a comma-separated list of OVSDB addresses into
// individual WithEndpoint options. This enables libovsdb's native failover
// when multiple endpoints are provided (e.g. "tcp:10.0.0.1:6641,tcp:10.0.0.2:6641").
func splitEndpoints(addr string) []client.Option {
	parts := strings.Split(addr, ",")
	opts := make([]client.Option, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			opts = append(opts, client.WithEndpoint(p))
		}
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

	return &OVNDatabases{NB: nbClient, SB: sbClient}, nil
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

// Close shuts down both NB and SB OVSDB clients.
func (d *OVNDatabases) Close() {
	d.NB.Close()
	d.SB.Close()
}
