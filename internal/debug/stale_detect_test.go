package debug

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaleDetector_DetectStaleMACBindings(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)
	d := &StaleDetector{SB: sbClient}
	ctx := context.Background()

	now := time.Now()
	// One binding older than the 24h cutoff, one fresh, one with no timestamp.
	testutil.InsertMACBinding(t, sbClient, "lp-old", "10.0.0.1", "00:00:00:00:00:01", int(now.Add(-25*time.Hour).UnixMilli()))
	testutil.InsertMACBinding(t, sbClient, "lp-new", "10.0.0.2", "00:00:00:00:00:02", int(now.UnixMilli()))
	testutil.InsertMACBinding(t, sbClient, "lp-zero", "10.0.0.3", "00:00:00:00:00:03", 0)

	entries, err := d.detectStaleMACBindings(ctx, defaultStaleMaxAge)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the >24h binding is stale")
	assert.Equal(t, "mac_binding", entries[0].Type)
	assert.Equal(t, "MAC_Binding", entries[0].Table)
	assert.Greater(t, entries[0].AgeSeconds, int64(0))
	assert.Equal(t, "10.0.0.1", entries[0].Details["ip"])
}

func TestStaleDetector_DetectOrphanedFDB(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)
	d := &StaleDetector{SB: sbClient}
	ctx := context.Background()

	// A port binding provides a valid port key; read the assigned tunnel key.
	testutil.InsertPortBinding(t, sbClient, "pb-valid", "", nil)
	var pbs []sb.PortBinding
	require.NoError(t, sbClient.List(ctx, &pbs))
	require.Len(t, pbs, 1)
	validKey := pbs[0].TunnelKey

	testutil.InsertFDB(t, sbClient, "00:00:00:00:aa:aa", 1, validKey)           // matches a binding
	testutil.InsertFDB(t, sbClient, "00:00:00:00:bb:bb", 1, validKey+1_000_000) // orphaned

	entries, err := d.detectOrphanedFDB(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the FDB with no matching port binding is orphaned")
	assert.Equal(t, "fdb", entries[0].Type)
	assert.Equal(t, "00:00:00:00:bb:bb", entries[0].Details["mac"])
}

func TestStaleDetector_DetectOrphanedPortBindings(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	d := &StaleDetector{NB: nbClient, SB: sbClient}
	ctx := context.Background()

	// Known NB entities: a switch port "known-port" and a router port "lrp1"
	// (whose chassisredirect alias is "cr-lrp1").
	testutil.InsertSwitchWithPort(t, nbClient, "sw", "known-port")
	testutil.InsertGatewayRouter(t, nbClient, "gw", "lrp1", nil, nil)

	testutil.InsertPortBinding(t, sbClient, "known-port", "", nil)             // matches an LSP
	testutil.InsertPortBinding(t, sbClient, "cr-lrp1", "chassisredirect", nil) // matches the cr- rule
	testutil.InsertPortBinding(t, sbClient, "localnet-x", "localnet", nil)     // skipped type
	testutil.InsertPortBinding(t, sbClient, "orphan-port", "", nil)            // no NB entity

	entries, err := d.detectOrphanedPortBindings(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only orphan-port has no corresponding NB entity")
	assert.Equal(t, "port_binding", entries[0].Type)
	assert.Equal(t, "orphan-port", entries[0].Details["logical_port"])
}

func TestStaleDetector_DetectAll(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	d := &StaleDetector{NB: nbClient, SB: sbClient}
	ctx := context.Background()

	// One of each kind of stale finding.
	testutil.InsertMACBinding(t, sbClient, "lp-old", "10.0.0.1", "00:00:00:00:00:01", int(time.Now().Add(-30*time.Hour).UnixMilli()))
	testutil.InsertFDB(t, sbClient, "00:00:00:00:cc:cc", 1, 16_000_000) // no port binding has this key
	testutil.InsertPortBinding(t, sbClient, "orphan-port", "", nil)

	result, err := d.DetectAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, result.StaleMAC)
	assert.Equal(t, 1, result.OrphanedFDB)
	assert.Equal(t, 1, result.OrphanedPorts)
	assert.Equal(t, 3, result.Total)
	assert.Len(t, result.Entries, 3)
}

func TestStaleDetector_DetectAllEmpty(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	d := &StaleDetector{NB: nbClient, SB: sbClient}

	result, err := d.DetectAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
	assert.Empty(t, result.Entries)
}
