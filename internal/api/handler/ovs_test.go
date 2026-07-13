package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/b42labs/northwatch/internal/ovshealth"
	"github.com/b42labs/northwatch/internal/testutil"
)

// listErrClient is a client.Client whose cache List always fails. Only List is
// overridden; every other method is left to the embedded nil interface, which
// the fleet-health path never calls.
type listErrClient struct {
	client.Client
	err error
}

func (c listErrClient) List(context.Context, any) error { return c.err }

// fakeOVSPool is a minimal ovsPool for the fleet-health tests: it returns fixed
// membership and looks clients up by system-id, so a connected chassis can be
// backed by a client whose List fails.
type fakeOVSPool struct {
	members []ovndb.OVSMemberStatus
	clients map[string]client.Client
}

func (p *fakeOVSPool) Members() []ovndb.OVSMemberStatus { return p.members }

func (p *fakeOVSPool) Client(systemID string) (client.Client, bool) {
	c, ok := p.clients[systemID]
	return c, ok
}

// countingOVSPool is an empty ovsPool that records how many times its cache was
// read, so a test can assert the health snapshot is served from cache within the
// TTL instead of recomputed.
type countingOVSPool struct{ membersCalls int }

func (p *countingOVSPool) Members() []ovndb.OVSMemberStatus {
	p.membersCalls++
	return nil
}

func (p *countingOVSPool) Client(string) (client.Client, bool) { return nil, false }

// setupOVS builds a pool with one connected chassis ("chassis-1") seeded with a
// br-int bridge and a vnet0 interface, and returns the registered mux.
func setupOVS(t *testing.T) *http.ServeMux {
	t.Helper()
	sock, seed := testutil.SetupOVSTestServer(t)
	testutil.InsertOVSBridgeWithInterface(t, seed, "br-int", "vnet0", map[string]int{"rx_packets": 7, "tx_packets": 9}, "up")

	return ovsMuxForSock(t, sock, func(c client.Client) bool {
		var ifaces []vs.Interface
		if err := c.List(context.Background(), &ifaces); err != nil {
			return false
		}
		return len(ifaces) == 1
	})
}

// ovsMuxForSock dials a "chassis-1" pool at the given OVS socket, waits until it
// is connected and the ready predicate holds against its monitored cache, then
// returns the registered mux. It isolates the pool wiring shared by the seeded
// fixtures.
func ovsMuxForSock(t *testing.T, sock string, ready func(client.Client) bool) *http.ServeMux {
	t.Helper()
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Add("chassis-1", "unix:"+sock))

	c, ok := pool.Client("chassis-1")
	require.True(t, ok)
	require.Eventually(t, c.Connected, 5*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return ready(c) }, 5*time.Second, 20*time.Millisecond)

	mux := http.NewServeMux()
	RegisterOVS(mux, pool)
	return mux
}

func ovsGet(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestOVSListInterface(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface")
	require.Equal(t, http.StatusOK, w.Code)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	// The live telemetry fields the SB DB cannot provide must be present.
	assert.Contains(t, rows[0], "statistics")
	assert.Contains(t, rows[0], "link_state")
	assert.Contains(t, rows[0], "error")
	assert.Equal(t, "vnet0", rows[0]["name"])
}

func TestOVSGetInterface(t *testing.T) {
	mux := setupOVS(t)
	list := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	uuid, _ := rows[0]["_uuid"].(string)
	require.NotEmpty(t, uuid)

	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface/"+uuid)
	require.Equal(t, http.StatusOK, w.Code)
	var row map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &row))
	assert.Equal(t, "vnet0", row["name"])
}

func TestOVSListBridge(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/bridge")
	require.Equal(t, http.StatusOK, w.Code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "br-int", rows[0]["name"])
	assert.Contains(t, rows[0], "datapath_type")
}

func TestOVSFleet(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs")
	require.Equal(t, http.StatusOK, w.Code)
	var members []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &members))
	require.Len(t, members, 1)
	assert.Equal(t, "chassis-1", members[0]["system_id"])
	assert.Equal(t, true, members[0]["connected"])
}

func TestOVSFleetHealth(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/health")
	require.Equal(t, http.StatusOK, w.Code)

	var health map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &health))
	// setupOVS seeds one connected chassis with one healthy (link_state=up)
	// bridge/port/interface, so the fleet is fully healthy.
	assert.EqualValues(t, 1, health["chassis"])
	assert.EqualValues(t, 1, health["connected"])
	assert.EqualValues(t, 0, health["unreachable"])
	assert.EqualValues(t, 1, health["bridges"])
	assert.EqualValues(t, 1, health["ports"])
	assert.EqualValues(t, 1, health["interfaces"])
	assert.EqualValues(t, 0, health["down_interfaces"])
	assert.EqualValues(t, 0, health["error_interfaces"])

	members, ok := health["members"].([]any)
	require.True(t, ok)
	require.Len(t, members, 1)
	member := members[0].(map[string]any)
	assert.Equal(t, "chassis-1", member["system_id"])
	assert.Equal(t, true, member["connected"])
}

func TestOVSFleetHealthDownAndDeltaBaseline(t *testing.T) {
	sock, seed := testutil.SetupOVSTestServer(t)
	// A single interface that is down (link_state=down) and carries a non-zero
	// lifetime rx_errors counter. Down is a level signal (counted immediately);
	// erroring is delta-based, so the first read is a baseline and does not flag
	// the lifetime counter as erroring.
	testutil.InsertOVSBridgeWithInterface(t, seed, "br-int", "vnet0", map[string]int{"rx_errors": 3}, "down")

	mux := ovsMuxForSock(t, sock, func(c client.Client) bool {
		var ifaces []vs.Interface
		if err := c.List(context.Background(), &ifaces); err != nil {
			return false
		}
		return len(ifaces) == 1
	})

	w := ovsGet(t, mux, "/api/v1/ovs/health")
	require.Equal(t, http.StatusOK, w.Code)

	var health map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &health))
	assert.EqualValues(t, 1, health["connected"])
	assert.EqualValues(t, 1, health["interfaces"])
	assert.EqualValues(t, 1, health["down_interfaces"])
	assert.EqualValues(t, 0, health["error_interfaces"], "a lifetime error counter must not flag on the first read")
	assert.EqualValues(t, 0, health["drop_interfaces"])
}

func TestOVSFleetHealthUnreachable(t *testing.T) {
	// A registered-but-unreachable chassis is listed but excluded from the
	// totals — proving a partial outage is never counted as healthy.
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Add("down", "unix:/nonexistent/northwatch-ovs.sock"))

	c, ok := pool.Client("down")
	require.True(t, ok)
	require.False(t, c.Connected())

	mux := http.NewServeMux()
	RegisterOVS(mux, pool)
	w := ovsGet(t, mux, "/api/v1/ovs/health")
	require.Equal(t, http.StatusOK, w.Code)

	var health map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &health))
	assert.EqualValues(t, 1, health["chassis"])
	assert.EqualValues(t, 0, health["connected"])
	assert.EqualValues(t, 1, health["unreachable"])
	assert.EqualValues(t, 0, health["interfaces"])

	members, ok := health["members"].([]any)
	require.True(t, ok)
	require.Len(t, members, 1)
	member := members[0].(map[string]any)
	assert.Equal(t, "down", member["system_id"])
	assert.Equal(t, false, member["connected"])
}

func TestOVSFleetHealthChassisListErrorExcluded(t *testing.T) {
	// A connected chassis whose cache read fails must be excluded from the fleet
	// — logged and dropped, aggregated as unreachable — not aborted into a
	// fleet-wide 500 that discards every other chassis's health.
	pool := &fakeOVSPool{
		members: []ovndb.OVSMemberStatus{{SystemID: "bad", Connected: true}},
		clients: map[string]client.Client{
			"bad": listErrClient{err: errors.New("cache read failed")},
		},
	}

	fh := ovsFleetHealth(context.Background(), pool, ovshealth.NewTracker())

	assert.Equal(t, 1, fh.Chassis)
	assert.Equal(t, 0, fh.Connected)
	assert.Equal(t, 1, fh.Unreachable)
	assert.Equal(t, 0, fh.Interfaces)
	require.Len(t, fh.Members, 1)
	assert.Equal(t, "bad", fh.Members[0].SystemID)
	assert.False(t, fh.Members[0].Connected)
	assert.Equal(t, 0, fh.Members[0].Interfaces)
}

func TestOVSHealthCacheCoalesces(t *testing.T) {
	// Within the TTL the snapshot is served from cache: a second call must not
	// re-read the pool, so a burst of /ovs/health polls collapses to one fan-out.
	pool := &countingOVSPool{}
	var hc healthCache
	base := time.Now()

	hc.get(context.Background(), pool, base)
	hc.get(context.Background(), pool, base.Add(ovsHealthTTL/2))
	assert.Equal(t, 1, pool.membersCalls, "second call within TTL must be served from cache")

	hc.get(context.Background(), pool, base.Add(2*ovsHealthTTL))
	assert.Equal(t, 2, pool.membersCalls, "call past TTL must recompute")
}

// ctxObservingClient records the context error seen inside List, so a test can
// assert the fleet-health fan-out ran on a live (non-cancelled) context.
type ctxObservingClient struct {
	client.Client
	sawErr error
}

func (c *ctxObservingClient) List(ctx context.Context, _ any) error {
	c.sawErr = ctx.Err()
	return nil
}

func TestOVSHealthCacheDetachedContext(t *testing.T) {
	// A caller whose context is already cancelled must still get a complete
	// snapshot: the compute runs on a context detached from cancellation, so the
	// per-chassis cache read is not aborted mid-fan-out.
	obs := &ctxObservingClient{}
	pool := &fakeOVSPool{
		members: []ovndb.OVSMemberStatus{{SystemID: "s1", Connected: true}},
		clients: map[string]client.Client{"s1": obs},
	}
	var hc healthCache

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // caller has already disconnected

	fh := hc.get(ctx, pool, time.Now())

	assert.NoError(t, obs.sawErr, "fan-out must run on a live context, not the cancelled caller context")
	assert.Equal(t, 1, fh.Chassis)
	assert.Equal(t, 1, fh.Connected)
	assert.Equal(t, 0, fh.Unreachable)
}

// gatedMembersPool blocks its second Members() call (the recompute fan-out)
// until release is closed, signalling on started when it begins blocking, so a
// test can prove a cached read is served while a recompute is in flight.
type gatedMembersPool struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (p *gatedMembersPool) Members() []ovndb.OVSMemberStatus {
	if p.calls.Add(1) == 2 {
		close(p.started)
		<-p.release
	}
	return nil
}

func (p *gatedMembersPool) Client(string) (client.Client, bool) { return nil, false }

func TestOVSHealthCacheServesCachedDuringRecompute(t *testing.T) {
	// The lock is not held across the fan-out, so a cached read must return
	// while a stale recompute is still blocked — no lock convoy.
	pool := &gatedMembersPool{started: make(chan struct{}), release: make(chan struct{})}
	var hc healthCache
	base := time.Now()

	// Prime the cache (fan-out #1, not gated).
	hc.get(context.Background(), pool, base)

	// Trigger a stale recompute (fan-out #2) that blocks inside Members().
	recomputeDone := make(chan ovshealth.FleetHealth, 1)
	go func() {
		recomputeDone <- hc.get(context.Background(), pool, base.Add(2*ovsHealthTTL))
	}()
	<-pool.started // the recompute is now in flight and blocked

	// A read within the TTL of the primed snapshot must return immediately; if
	// the lock were held across the fan-out this would deadlock (test timeout).
	hc.get(context.Background(), pool, base.Add(ovsHealthTTL/2))

	// The recompute must still be blocked — the cached read did not wait on it.
	select {
	case <-recomputeDone:
		t.Fatal("recompute completed before release; gating is broken")
	default:
	}

	close(pool.release)
	<-recomputeDone
}

func TestOVSUnknownChassis(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/nope/interface")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOVSUnknownTable(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/bogus")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOVSRowNotFound(t *testing.T) {
	mux := setupOVS(t)
	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/interface/00000000-0000-0000-0000-000000000000")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOVSNewTablesRoutable(t *testing.T) {
	mux := setupOVS(t)
	// The tables widened beyond the original interface/bridge/port/open-vswitch/
	// manager/controller set: each must be routable (200 + JSON array), not 404.
	slugs := []string{
		"ipfix", "sflow", "netflow", "mirror", "qos", "queue",
		"ct-zone", "ct-timeout-policy", "datapath",
		"flow-table", "flow-sample-collector-set", "ssl", "autoattach",
	}
	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/"+slug)
			require.Equal(t, http.StatusOK, w.Code)
			var rows []map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
		})
	}
}

func TestOVSSSLRedactsPrivateKey(t *testing.T) {
	sock, seed := testutil.SetupOVSTestServer(t)
	testutil.InsertOVSSSLRow(t, seed, "SECRET-PRIVATE-KEY", "PUBLIC-CERT", "CA-CERT")

	mux := ovsMuxForSock(t, sock, func(c client.Client) bool {
		var rows []vs.SSL
		if err := c.List(context.Background(), &rows); err != nil {
			return false
		}
		return len(rows) == 1
	})

	w := ovsGet(t, mux, "/api/v1/ovs/chassis-1/ssl")
	require.Equal(t, http.StatusOK, w.Code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	// The sensitive key material must never reach the client, under any key;
	// public certs stay.
	assert.NotContains(t, w.Body.String(), "SECRET-PRIVATE-KEY")
	assert.NotContains(t, rows[0], "private_key")
	assert.Equal(t, "PUBLIC-CERT", rows[0]["certificate"])
	assert.Equal(t, "CA-CERT", rows[0]["ca_cert"])
}

func TestOVSUnreachable(t *testing.T) {
	// A registered-but-unreachable chassis serves 503, not 404 or a cache read.
	model, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	pool := ovndb.NewOVSPool(model, nil)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Add("down", "unix:/nonexistent/northwatch-ovs.sock"))

	c, ok := pool.Client("down")
	require.True(t, ok)
	require.False(t, c.Connected())

	mux := http.NewServeMux()
	RegisterOVS(mux, pool)
	w := ovsGet(t, mux, "/api/v1/ovs/down/interface")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
