package ovsdb_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/b42labs/northwatch/internal/testutil"
)

// waitMonitored blocks until the member's initial MonitorAll has landed, which
// the seeded interface appearing in its cache proves.
//
// Waiting on Connected() alone is not enough, and not only because it asserts
// less: OVSPool.connectAndMonitor runs Connect and MonitorAll under one
// cancellable context, so a test that returns as soon as the client reports
// Connected lets its deferred pool.Close cancel a monitor call that is still in
// flight. libovsdb v0.8.1 then reads the RPC reply that rpc2 is concurrently
// unmarshalling into (client.go:996 — the read is discarded, but it is a read),
// and the race detector fails the package. See #109; the fix belongs upstream,
// so until it lands do not narrow these waits back to Connected().
func waitMonitored(t *testing.T, c client.Client) {
	t.Helper()
	require.Eventually(t, func() bool {
		// Connected() must gate the List: libovsdb builds the per-database API
		// during Connect, so List on a member that has not connected yet nil-derefs
		// (client.go:1420).
		if !c.Connected() {
			return false
		}
		var ifaces []vs.Interface
		if err := c.List(context.Background(), &ifaces); err != nil {
			return false
		}
		return len(ifaces) == 1
	}, 5*time.Second, 20*time.Millisecond)
}

func TestPoolReachable(t *testing.T) {
	sock, seed := testutil.SetupOVSTestServer(t)
	testutil.InsertOVSBridgeWithInterface(t, seed, "br-int", "vnet0", map[string]int{"rx_packets": 42}, "up")

	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	defer pool.Close()

	require.NoError(t, pool.Add("chassis-1", "unix:"+sock))

	c, ok := pool.Client("chassis-1")
	require.True(t, ok)
	waitMonitored(t, c)

	var ifaces []vs.Interface
	require.NoError(t, c.List(context.Background(), &ifaces))
	require.Len(t, ifaces, 1)
	assert.Equal(t, "vnet0", ifaces[0].Name)
	assert.Equal(t, 42, ifaces[0].Statistics["rx_packets"])
}

func TestPoolPartialOutage(t *testing.T) {
	// One reachable chassis plus one with an unreachable address: the reachable
	// member must serve data while the unreachable one stays Connected:false and
	// keeps retrying in its own goroutine without affecting the good one.
	sock, seed := testutil.SetupOVSTestServer(t)
	testutil.InsertOVSBridgeWithInterface(t, seed, "br-int", "vnet0", nil, "up")

	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	defer pool.Close()

	require.NoError(t, pool.Add("good", "unix:"+sock))
	require.NoError(t, pool.Add("bad", "unix:/nonexistent/northwatch-ovs.sock"))

	good, ok := pool.Client("good")
	require.True(t, ok)
	// The unreachable member is safe to cancel mid-flight — it never gets past
	// Connect, so no monitor reply is in play. Only "good" needs waitMonitored.
	waitMonitored(t, good)

	bad, ok := pool.Client("bad")
	require.True(t, ok)
	assert.False(t, bad.Connected())

	statuses := pool.Members()
	require.Len(t, statuses, 2)
	byID := map[string]bool{}
	for _, s := range statuses {
		byID[s.SystemID] = s.Connected
	}
	assert.True(t, byID["good"])
	assert.False(t, byID["bad"])
}

func TestMembersOmitsAddr(t *testing.T) {
	// The unauthenticated GET /api/v1/ovs endpoint serializes Members() verbatim.
	// The dialable management address must never leak into that output — it is the
	// OVSDB control-plane map of the fleet and a lateral-movement target.
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	defer pool.Close()

	const addr = "unix:/nonexistent/northwatch-ovs.sock"
	require.NoError(t, pool.Add("node-7", addr))

	data, err := json.Marshal(pool.Members())
	require.NoError(t, err)
	assert.NotContains(t, string(data), "addr")
	assert.NotContains(t, string(data), "northwatch-ovs.sock")
}

func TestPoolDuplicateAdd(t *testing.T) {
	sock, _ := testutil.SetupOVSTestServer(t)
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	defer pool.Close()

	require.NoError(t, pool.Add("chassis-1", "unix:"+sock))
	err = pool.Add("chassis-1", "unix:"+sock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
	assert.Len(t, pool.Members(), 1)
}

func TestPoolClientUnknown(t *testing.T) {
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	defer pool.Close()

	c, ok := pool.Client("nope")
	assert.False(t, ok)
	assert.Nil(t, c)
}

func TestPoolCloseStops(t *testing.T) {
	// Close must cancel the supervisor of a never-reachable member and return
	// cleanly (no goroutine leak — the race detector and wg.Wait guard this).
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	require.NoError(t, pool.Add("bad", "unix:/nonexistent/northwatch-ovs.sock"))

	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return — supervisor goroutine leaked")
	}
}

func TestConnectPool(t *testing.T) {
	sock, seed := testutil.SetupOVSTestServer(t)
	testutil.InsertOVSBridgeWithInterface(t, seed, "br-int", "vnet0", nil, "up")

	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)

	pool := ovndb.ConnectOVSPool(model, nil, map[string]string{
		"good": "unix:" + sock,
	})
	defer pool.Close()

	require.Len(t, pool.Members(), 1)
	c, ok := pool.Client("good")
	require.True(t, ok)
	waitMonitored(t, c)
}
