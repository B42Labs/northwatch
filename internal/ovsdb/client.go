package ovsdb

import (
	"context"
	"fmt"
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
// On any failure, it closes the partially-opened clients before returning.
func Connect(ctx context.Context, nbAddr, sbAddr string, nbModel, sbModel model.ClientDBModel) (*OVNDatabases, error) {
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
		nbErr = connectAndMonitor(ctx, nbClient, nbAddr)
	}()
	go func() {
		defer wg.Done()
		sbErr = connectAndMonitor(ctx, sbClient, sbAddr)
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

func connectAndMonitor(ctx context.Context, c client.Client, addr string) error {
	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to %s: %w", addr, err)
	}

	if _, err := c.MonitorAll(ctx); err != nil {
		c.Close()
		return fmt.Errorf("monitoring %s: %w", addr, err)
	}

	return nil
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
