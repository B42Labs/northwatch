package ovnsim

import (
	"context"
	"fmt"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/cenkalti/backoff/v4"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// newBackoff returns an exponential backoff that retries forever, matching the
// reconnect behaviour used by the main Northwatch client.
func newBackoff() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 0
	return bo
}

// ConnectNB dials the OVN Northbound OVSDB server at addr (e.g.
// "tcp:127.0.0.1:6641"), populates the libovsdb cache with a MonitorAll and
// returns a ready client plus a close function. The cache is required: the
// model-based Create API and all of ovnsim's idempotency/selection reads work
// against it.
func ConnectNB(ctx context.Context, addr string) (client.Client, func(), error) {
	dbModel, err := nb.FullDatabaseModel()
	if err != nil {
		return nil, nil, fmt.Errorf("building NB model: %w", err)
	}

	c, err := client.NewOVSDBClient(dbModel,
		client.WithEndpoint(addr),
		client.WithReconnect(5*time.Second, newBackoff()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating NB client: %w", err)
	}

	if err := c.Connect(ctx); err != nil {
		c.Close()
		return nil, nil, fmt.Errorf("connecting to NB at %s: %w", addr, err)
	}

	if _, err := c.MonitorAll(ctx); err != nil {
		c.Close()
		return nil, nil, fmt.Errorf("monitoring NB at %s: %w", addr, err)
	}

	return c, c.Close, nil
}
