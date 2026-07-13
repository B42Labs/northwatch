package snapshotsession_test

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/cluster"
	"github.com/b42labs/northwatch/internal/history"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/snapshotsession"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManager wires a Manager backed by store with an in-memory-server build
// func, returning it alongside the registry.
func newTestManager(t *testing.T, store *history.Store) (*snapshotsession.Manager, *cluster.Registry) {
	t.Helper()
	ctx := context.Background()
	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	reg := cluster.NewRegistry()
	build := func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
		m1, _ := nb.FullDatabaseModel()
		m2, _ := sb.FullDatabaseModel()
		dbs, err := ovndb.Connect(ctx, nbAddr, sbAddr, m1, m2, ovndb.MonitorOptions{SkipServerMonitors: true}, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		return &cluster.Cluster{Name: name, Label: label, DBs: dbs}, nil, nil
	}
	mgr := snapshotsession.New(store, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		build,
		func(*cluster.Cluster) {},
		func(string) {},
		&fakeLive{},
	)
	t.Cleanup(mgr.Close)
	return mgr, reg
}

// storeSnapshot persists a one-row snapshot and returns its id. hexSuffix must be
// hex digits: it is embedded in the row's (valid) UUID.
func storeSnapshot(t *testing.T, store *history.Store, hexSuffix string) int64 {
	t.Helper()
	uuid := ("11111111-1111-4111-8111-000000000000")[:36-len(hexSuffix)] + hexSuffix
	rows := []history.SnapshotRow{{
		Database: "nb", Table: "Logical_Switch", UUID: uuid,
		Data: map[string]any{"_uuid": uuid, "name": "ls-" + hexSuffix, "external_ids": map[string]any{}, "other_config": map[string]any{}, "ports": []any{}},
	}}
	meta, err := store.CreateSnapshot(context.Background(), "manual", hexSuffix, rows)
	require.NoError(t, err)
	return meta.ID
}

func TestManagerCapRejectsExcessLoads(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	mgr, _ := newTestManager(t, store)
	mgr.SetMaxLoaded(1)

	id1 := storeSnapshot(t, store, "aaaa")
	id2 := storeSnapshot(t, store, "bbbb")

	_, err := mgr.Load(ctx, id1)
	require.NoError(t, err)

	_, err = mgr.Load(ctx, id2)
	assert.ErrorIs(t, err, snapshotsession.ErrTooManyLoaded)
}

func TestManagerUnloadDrainGrace(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	mgr, reg := newTestManager(t, store)
	mgr.SetDrainGrace(80 * time.Millisecond)

	id := storeSnapshot(t, store, "cccc")
	ld, err := mgr.Load(ctx, id)
	require.NoError(t, err)
	c, ok := reg.Get(ld.Cluster)
	require.True(t, ok)
	require.True(t, c.DBs.NB.Connected())

	require.NoError(t, mgr.Unload(ctx, id))
	// Backing client stays alive through the drain grace so in-flight requests finish.
	assert.True(t, c.DBs.NB.Connected(), "client must not close before the drain grace elapses")
	// After the grace it is torn down.
	require.Eventually(t, func() bool { return !c.DBs.NB.Connected() }, time.Second, 10*time.Millisecond)
}

func TestManagerCloseFlushesDrainImmediately(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	mgr, reg := newTestManager(t, store)
	mgr.SetDrainGrace(time.Hour) // long grace so only Close can flush it

	id := storeSnapshot(t, store, "dddd")
	ld, err := mgr.Load(ctx, id)
	require.NoError(t, err)
	c, ok := reg.Get(ld.Cluster)
	require.True(t, ok)

	require.NoError(t, mgr.Unload(ctx, id))
	require.True(t, c.DBs.NB.Connected(), "long grace means the client is still alive")

	mgr.Close()
	assert.False(t, c.DBs.NB.Connected(), "Close must flush pending drains synchronously")
}

func newStore(t *testing.T) *history.Store {
	t.Helper()
	store, err := history.NewStore(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestManagerLoadUnload drives a stored history snapshot through the full
// runtime path: export → reassemble → serve → register as a read-only cluster,
// then verify the data is queryable through that cluster and that Unload tears
// everything down.
func TestManagerLoadUnload(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	const lsUUID = "11111111-1111-4111-8111-111111111111"
	rows := []history.SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: lsUUID, Data: map[string]any{
			"_uuid":        lsUUID,
			"name":         "ls0",
			"external_ids": map[string]any{"owner": "team-net"},
			"other_config": map[string]any{},
			"ports":        []any{},
		}},
	}
	meta, err := store.CreateSnapshot(ctx, "manual", "test", rows)
	require.NoError(t, err)

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	reg := cluster.NewRegistry()
	registered := map[string]bool{}

	build := func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
		m1, _ := nb.FullDatabaseModel()
		m2, _ := sb.FullDatabaseModel()
		dbs, err := ovndb.Connect(ctx, nbAddr, sbAddr, m1, m2, ovndb.MonitorOptions{SkipServerMonitors: true}, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		return &cluster.Cluster{Name: name, Label: label, DBs: dbs}, nil, nil
	}

	live := &fakeLive{}

	mgr := snapshotsession.New(store, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		build,
		func(c *cluster.Cluster) { registered[c.Name] = true },
		func(name string) { delete(registered, name) },
		live,
	)
	defer mgr.Close()

	want := "snapshot-" + strconv.FormatInt(meta.ID, 10)

	ld, err := mgr.Load(ctx, meta.ID)
	require.NoError(t, err)
	assert.Equal(t, want, ld.Cluster)
	assert.Equal(t, "snapshot", ld.Mode)
	assert.Equal(t, meta.ID, ld.SourceID)
	assert.True(t, registered[want], "routes must be registered")
	assert.Equal(t, 1, live.suspends, "live OVN must be suspended on first load")
	assert.Equal(t, 0, live.resumes)

	c, ok := reg.Get(want)
	require.True(t, ok, "cluster must be in the registry")
	assert.Equal(t, "snapshot", c.Mode)
	require.NotNil(t, c.Snapshot)
	assert.Equal(t, meta.ID, c.Snapshot.SourceID)

	// The snapshot data is queryable through the loaded cluster's client.
	var switches []nb.LogicalSwitch
	require.NoError(t, c.DBs.NB.List(ctx, &switches))
	require.Len(t, switches, 1)
	assert.Equal(t, "ls0", switches[0].Name)
	assert.Equal(t, map[string]string{"owner": "team-net"}, switches[0].ExternalIDs)

	// Loading the same snapshot again is idempotent and does not re-suspend.
	ld2, err := mgr.Load(ctx, meta.ID)
	require.NoError(t, err)
	assert.Equal(t, ld.Cluster, ld2.Cluster)
	assert.Len(t, reg.List(), 1, "no duplicate cluster")
	assert.Equal(t, 1, live.suspends, "idempotent load must not suspend again")

	// Unload tears down registry entry and routing, and resumes live OVN.
	require.NoError(t, mgr.Unload(ctx, meta.ID))
	_, ok = reg.Get(want)
	assert.False(t, ok)
	assert.False(t, registered[want])
	assert.Equal(t, 1, live.resumes, "live OVN must be resumed when the last snapshot is ejected")

	// Unloading again reports not-loaded.
	assert.ErrorIs(t, mgr.Unload(ctx, meta.ID), snapshotsession.ErrNotLoaded)

	// Loading an unknown snapshot surfaces history.ErrNotFound.
	_, err = mgr.Load(ctx, 99999)
	assert.ErrorIs(t, err, history.ErrNotFound)
}

// TestManagerConcurrentLoadUnloadResumesLive guards the ordering race where a
// Load's live-monitor suspend and a concurrent Unload's resume execute out of
// order and leave the live OVN connection permanently suspended with no snapshot
// loaded (the default cluster goes dark). It blocks the Load inside
// SuspendMonitors, runs Unload concurrently, and asserts the live connection
// ends up resumed once both settle.
func TestManagerConcurrentLoadUnloadResumesLive(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	reg := cluster.NewRegistry()
	build := func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
		m1, _ := nb.FullDatabaseModel()
		m2, _ := sb.FullDatabaseModel()
		dbs, err := ovndb.Connect(ctx, nbAddr, sbAddr, m1, m2, ovndb.MonitorOptions{SkipServerMonitors: true}, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		return &cluster.Cluster{Name: name, Label: label, DBs: dbs}, nil, nil
	}

	live := &blockingLive{
		entered:  make(chan struct{}),
		released: make(chan struct{}),
		resumed:  make(chan struct{}),
	}
	mgr := snapshotsession.New(store, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		build,
		func(*cluster.Cluster) {},
		func(string) {},
		live,
	)
	t.Cleanup(mgr.Close)

	id := storeSnapshot(t, store, "eeee")

	loadDone := make(chan error, 1)
	go func() {
		_, err := mgr.Load(ctx, id)
		loadDone <- err
	}()

	// Wait until Load has committed the entry and blocked inside SuspendMonitors.
	<-live.entered

	unloadDone := make(chan error, 1)
	go func() { unloadDone <- mgr.Unload(ctx, id) }()

	// The buggy code lets Unload resume immediately while the suspend is still
	// blocked; wait for that premature resume so releasing the suspend below
	// re-suspends after it. The fixed code serializes the transition, so resume
	// cannot run until the suspend completes — fall through on the timeout.
	select {
	case <-live.resumed:
	case <-time.After(300 * time.Millisecond):
	}
	close(live.released)

	require.NoError(t, <-loadDone)
	require.NoError(t, <-unloadDone)

	assert.False(t, live.isSuspended(),
		"live OVN must be resumed after a load+unload settles, not left suspended")
}

// TestManagerUnloadCancelledContextResumesLive guards against a cancelled unload
// request leaving the live OVN connection suspended. SuspendMonitors purges both
// live caches, so an aborted ResumeMonitors would leave the live cluster dark
// with no later event to retry it: the manager must drive the monitor transition
// with a context the disconnecting client cannot cancel.
func TestManagerUnloadCancelledContextResumesLive(t *testing.T) {
	store := newStore(t)

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	reg := cluster.NewRegistry()
	build := func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
		m1, _ := nb.FullDatabaseModel()
		m2, _ := sb.FullDatabaseModel()
		dbs, err := ovndb.Connect(context.Background(), nbAddr, sbAddr, m1, m2, ovndb.MonitorOptions{SkipServerMonitors: true}, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		return &cluster.Cluster{Name: name, Label: label, DBs: dbs}, nil, nil
	}

	live := &cancelAwareLive{}
	mgr := snapshotsession.New(store, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		build,
		func(*cluster.Cluster) {},
		func(string) {},
		live,
	)
	t.Cleanup(mgr.Close)

	id := storeSnapshot(t, store, "ffff")
	_, err = mgr.Load(context.Background(), id)
	require.NoError(t, err)
	require.True(t, live.isSuspended(), "live must be suspended while a snapshot is loaded")

	// The operator's unload request is cancelled (client disconnect) before the
	// live monitors are resumed. The transition must still run to completion.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, mgr.Unload(ctx, id))

	assert.False(t, live.isSuspended(),
		"a cancelled unload request must not leave the live OVN connection suspended")
}

// cancelAwareLive is a LiveSource that honours context cancellation, so a test
// can prove a cancelled request does not abort the live-monitor transition.
type cancelAwareLive struct {
	mu        sync.Mutex
	suspended bool
}

func (c *cancelAwareLive) SuspendMonitors(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.suspended = true
	c.mu.Unlock()
	return nil
}

func (c *cancelAwareLive) ResumeMonitors(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.suspended = false
	c.mu.Unlock()
	return nil
}

func (c *cancelAwareLive) isSuspended() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.suspended
}

// fakeLive records SuspendMonitors/ResumeMonitors calls.
type fakeLive struct {
	suspends int
	resumes  int
}

func (f *fakeLive) SuspendMonitors(context.Context) error {
	f.suspends++
	return nil
}

func (f *fakeLive) ResumeMonitors(context.Context) error {
	f.resumes++
	return nil
}

// blockingLive is a LiveSource whose first SuspendMonitors blocks until released,
// so a test can interleave a concurrent Unload between a Load's commit and its
// suspend. It tracks the resulting suspended state and signals when a resume runs.
type blockingLive struct {
	entered  chan struct{} // closed once SuspendMonitors has been entered
	released chan struct{} // test closes this to let the blocked suspend finish
	resumed  chan struct{} // closed once ResumeMonitors has run

	enterOnce  sync.Once
	resumeOnce sync.Once

	mu        sync.Mutex
	suspended bool
}

func (b *blockingLive) SuspendMonitors(context.Context) error {
	b.enterOnce.Do(func() {
		close(b.entered)
		<-b.released
	})
	b.mu.Lock()
	b.suspended = true
	b.mu.Unlock()
	return nil
}

func (b *blockingLive) ResumeMonitors(context.Context) error {
	b.mu.Lock()
	b.suspended = false
	b.mu.Unlock()
	b.resumeOnce.Do(func() { close(b.resumed) })
	return nil
}

func (b *blockingLive) isSuspended() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.suspended
}
