package ovsdb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
)

type OVNDatabases struct {
	NB client.Client
	SB client.Client
}

func newBackoff() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 0 // retry forever
	return bo
}

func Connect(ctx context.Context, nbAddr, sbAddr string, nbModel, sbModel model.ClientDBModel) (*OVNDatabases, error) {
	// Create clients sequentially to avoid race in libovsdb's stdr.SetVerbosity.
	// Each client gets its own backoff instance since ExponentialBackOff is stateful.
	nbClient, err := client.NewOVSDBClient(
		nbModel,
		client.WithEndpoint(nbAddr),
		client.WithReconnect(10*time.Second, newBackoff()),
	)
	if err != nil {
		return nil, fmt.Errorf("creating NB client: %w", err)
	}

	sbClient, err := client.NewOVSDBClient(
		sbModel,
		client.WithEndpoint(sbAddr),
		client.WithReconnect(10*time.Second, newBackoff()),
	)
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

func (d *OVNDatabases) Ready() bool {
	return d.NB.Connected() && d.SB.Connected()
}

func (d *OVNDatabases) Close() {
	d.NB.Close()
	d.SB.Close()
}
