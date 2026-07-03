// Package snapshotsession loads stored history snapshots as live, switchable
// data sources at runtime. A loaded snapshot is served by in-memory OVSDB
// servers and registered as an additional read-only cluster, so the UI can
// switch to it (and back) without restarting Northwatch.
package snapshotsession

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"

	"github.com/b42labs/northwatch/internal/cluster"
	"github.com/b42labs/northwatch/internal/history"
	"github.com/b42labs/northwatch/internal/snapshot"
)

// ErrNotLoaded is returned when unloading a snapshot that is not currently loaded.
var ErrNotLoaded = fmt.Errorf("snapshot not loaded")

// ErrTooManyLoaded is returned by Load when the session cap is reached. Each
// loaded snapshot keeps two in-memory OVSDB servers plus full client caches
// resident until unloaded, so unbounded loads would OOM.
var ErrTooManyLoaded = fmt.Errorf("too many snapshots loaded")

// ErrLoadInProgress is returned when a Load for the same snapshot is already
// running (its slow build has not yet committed).
var ErrLoadInProgress = fmt.Errorf("snapshot load already in progress")

// DefaultMaxLoaded caps the number of concurrently loaded snapshot sessions.
const DefaultMaxLoaded = 4

// DefaultDrainGrace is how long a snapshot's in-memory servers and clients are
// kept alive after it is unregistered, so in-flight requests already dispatched
// by the proxy can drain before their backing state is torn down.
const DefaultDrainGrace = 2 * time.Second

// LiveSource is the live OVN data source whose monitors are suspended while any
// snapshot is loaded and resumed once the last one is ejected. It is satisfied
// by *ovsdb.OVNDatabases. It may be nil (then live is never touched).
type LiveSource interface {
	SuspendMonitors(ctx context.Context) error
	ResumeMonitors(ctx context.Context) error
}

// BuildClusterFunc connects clients to the in-memory servers backing a snapshot
// and assembles a read-only cluster. It returns the cluster plus any cleanup
// functions to run on unload (typically none for a static snapshot).
type BuildClusterFunc func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error)

// Loaded describes a snapshot that has been loaded as a cluster.
type Loaded struct {
	Cluster   string
	Label     string
	Mode      string
	SourceID  int64
	CreatedAt string
}

// Manager loads and unloads history snapshots as runtime clusters. It is safe
// for concurrent use.
type Manager struct {
	store    *history.Store
	reg      *cluster.Registry
	nbModel  model.ClientDBModel
	sbModel  model.ClientDBModel
	nbSchema ovsdb.DatabaseSchema
	sbSchema ovsdb.DatabaseSchema

	build      BuildClusterFunc
	register   func(c *cluster.Cluster) // build a sub-mux and route to the cluster
	unregister func(name string)        // stop routing to the cluster
	live       LiveSource               // live OVN, suspended while snapshots are loaded (may be nil)

	maxLoaded  int
	drainGrace time.Duration

	mu            sync.Mutex
	loaded        map[int64]*entry
	loading       map[int64]struct{} // ids whose slow build is in flight
	pendingDrains map[int64]func()   // deferred resource-close tails, keyed by sequence
	drainSeq      int64

	// liveMu serializes live-monitor transitions so a suspend from one goroutine
	// cannot win a race against a resume from another. liveSuspended (guarded by
	// liveMu) records whether the live monitors are currently suspended.
	liveMu        sync.Mutex
	liveSuspended bool
}

type entry struct {
	loaded Loaded
	// stop tears down routing and registry membership; run immediately on unload.
	stop []func()
	// drain closes the clients and in-memory servers; deferred past the drain
	// grace so in-flight requests finish first.
	drain []func()
}

// New creates a Manager. register/unregister hook the cluster's HTTP routes into
// (and out of) the cluster proxy; build assembles the read-only cluster.
func New(
	store *history.Store,
	reg *cluster.Registry,
	nbModel, sbModel model.ClientDBModel,
	nbSchema, sbSchema ovsdb.DatabaseSchema,
	build BuildClusterFunc,
	register func(c *cluster.Cluster),
	unregister func(name string),
	live LiveSource,
) *Manager {
	return &Manager{
		store:         store,
		reg:           reg,
		nbModel:       nbModel,
		sbModel:       sbModel,
		nbSchema:      nbSchema,
		sbSchema:      sbSchema,
		build:         build,
		register:      register,
		unregister:    unregister,
		live:          live,
		maxLoaded:     DefaultMaxLoaded,
		drainGrace:    DefaultDrainGrace,
		loaded:        make(map[int64]*entry),
		loading:       make(map[int64]struct{}),
		pendingDrains: make(map[int64]func()),
	}
}

// SetMaxLoaded overrides the concurrent-session cap. A value <= 0 is ignored.
func (m *Manager) SetMaxLoaded(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n > 0 {
		m.maxLoaded = n
	}
}

// SetDrainGrace overrides how long backing resources are kept alive after a
// snapshot is unregistered. A value < 0 is ignored (0 tears down immediately).
func (m *Manager) SetDrainGrace(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d >= 0 {
		m.drainGrace = d
	}
}

// clusterName is the synthetic registry name for a loaded snapshot. It is
// derived 1:1 from the history snapshot ID.
func clusterName(id int64) string {
	return "snapshot-" + strconv.FormatInt(id, 10)
}

// Load materializes history snapshot id into in-memory OVSDB servers, registers
// it as a read-only cluster, and starts routing requests to it. Loading the same
// snapshot again is idempotent and returns the existing cluster.
func (m *Manager) Load(ctx context.Context, id int64) (Loaded, error) {
	// Reserve the load under the lock, then do the slow build (export + assemble
	// + two 5s ready-waits + a 30s connect) OUTSIDE the lock so one hung load
	// cannot block every other load/unload for ~40s.
	m.mu.Lock()
	if e, ok := m.loaded[id]; ok {
		m.mu.Unlock()
		return e.loaded, nil
	}
	if _, ok := m.loading[id]; ok {
		m.mu.Unlock()
		return Loaded{}, ErrLoadInProgress
	}
	if len(m.loaded)+len(m.loading) >= m.maxLoaded {
		max := m.maxLoaded
		loaded := len(m.loaded)
		m.mu.Unlock()
		return Loaded{}, fmt.Errorf("%w: %d already loaded (max %d)", ErrTooManyLoaded, loaded, max)
	}
	m.loading[id] = struct{}{}
	m.mu.Unlock()

	// Always clear the in-flight placeholder, whatever the outcome.
	defer func() {
		m.mu.Lock()
		delete(m.loading, id)
		m.mu.Unlock()
	}()

	export, err := m.store.ExportSnapshot(ctx, id)
	if err != nil {
		return Loaded{}, err // wraps history.ErrNotFound for the unknown-id case
	}

	rows := make([]snapshot.RowInput, len(export.Rows))
	for i, r := range export.Rows {
		rows[i] = snapshot.RowInput{Database: r.Database, Table: r.Table, UUID: r.UUID, Data: r.Data}
	}
	file, err := snapshot.BuildFromRows(m.nbModel, m.sbModel, m.nbSchema, m.sbSchema, rows)
	if err != nil {
		return Loaded{}, fmt.Errorf("building snapshot %d: %w", id, err)
	}

	servers, err := snapshot.Serve(file, m.nbModel, m.sbModel, m.nbSchema, m.sbSchema)
	if err != nil {
		return Loaded{}, fmt.Errorf("serving snapshot %d: %w", id, err)
	}

	name := clusterName(id)
	createdAt := export.Meta.Timestamp.UTC().Format(time.RFC3339)
	label := snapshotLabel(export.Meta)

	c, stops, err := m.build(name, label, servers.NBAddr, servers.SBAddr)
	if err != nil {
		servers.Close()
		return Loaded{}, fmt.Errorf("building snapshot cluster %d: %w", id, err)
	}
	c.Mode = "snapshot"
	c.Snapshot = &cluster.SnapshotMeta{SourceID: id, CreatedAt: createdAt}

	ld := Loaded{Cluster: name, Label: label, Mode: "snapshot", SourceID: id, CreatedAt: createdAt}

	// Commit under the lock and start routing.
	m.mu.Lock()
	if e, ok := m.loaded[id]; ok {
		// Raced with another commit of the same id (shouldn't happen while
		// loading[id] is held, but stay safe): discard the partial build.
		m.mu.Unlock()
		for _, s := range stops {
			s()
		}
		c.DBs.Close()
		servers.Close()
		return e.loaded, nil
	}

	m.reg.Register(name, c)
	m.register(c)

	stop := []func(){
		func() { m.unregister(name) },
		func() { m.reg.Unregister(name) },
	}
	drain := append([]func(){}, stops...)
	drain = append(drain, c.DBs.Close, servers.Close)

	m.loaded[id] = &entry{loaded: ld, stop: stop, drain: drain}
	m.mu.Unlock()

	// Suspend the live OVN connection so it stops streaming updates while the
	// operator works on the snapshot. reconcileLive serializes the transition so
	// a concurrent Unload cannot resume the live connection before this suspend
	// runs and leave it permanently dark.
	m.reconcileLive(ctx)

	return ld, nil
}

// Unload removes a loaded snapshot cluster and frees its resources. When the
// last loaded snapshot is removed, the live OVN connection is resumed and its
// tables reloaded.
func (m *Manager) Unload(ctx context.Context, id int64) error {
	m.mu.Lock()
	e, ok := m.loaded[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotLoaded
	}
	delete(m.loaded, id)

	// Stop routing and drop from the registry immediately so no new request
	// reaches the cluster.
	for _, fn := range e.stop {
		fn()
	}

	// Defer closing the clients/servers past the drain grace so any request the
	// proxy already dispatched finishes against live backing state. sync.OnceFunc
	// makes a later Close() call a no-op if the timer already fired.
	drainTail := sync.OnceFunc(func() {
		for _, fn := range e.drain {
			fn()
		}
	})
	seq := m.drainSeq
	m.drainSeq++
	m.pendingDrains[seq] = drainTail
	grace := m.drainGrace
	m.mu.Unlock()

	// Resume the live OVN connection once the last snapshot is ejected.
	// reconcileLive serializes the transition against a concurrent Load's
	// suspend so the live connection is never left suspended with no snapshot.
	m.reconcileLive(ctx)

	time.AfterFunc(grace, func() {
		drainTail()
		m.mu.Lock()
		delete(m.pendingDrains, seq)
		m.mu.Unlock()
	})
	return nil
}

// reconcileLive drives the live OVN monitors to match whether any snapshot is
// loaded: suspended while at least one is loaded, resumed once the last one is
// ejected. liveMu serializes the transition and each call re-reads the
// authoritative loaded count under it, so a stale suspend from a racing Load
// cannot win over a newer resume from a concurrent Unload (or vice versa). The
// monitor RPC runs without m.mu held so a slow transition cannot block every
// other load/unload.
func (m *Manager) reconcileLive(ctx context.Context) {
	if m.live == nil {
		return
	}
	m.liveMu.Lock()
	defer m.liveMu.Unlock()

	m.mu.Lock()
	wantSuspended := len(m.loaded) > 0
	m.mu.Unlock()

	if wantSuspended == m.liveSuspended {
		return
	}

	// The monitor transition must run to completion even if the HTTP request that
	// triggered it is cancelled. SuspendMonitors has already purged both live
	// caches, so an aborted ResumeMonitors would leave the live cluster dark
	// (empty caches, never ready) with no later event to retry it. Detach from the
	// caller's cancellation — keeping any request-scoped values — so a
	// disconnecting client cannot wedge the live connection.
	ctx = context.WithoutCancel(ctx)

	if wantSuspended {
		slog.Info("snapshot loaded — suspending live OVN connection")
		if err := m.live.SuspendMonitors(ctx); err != nil {
			slog.Error("suspending live OVN monitors failed", "err", err)
			return
		}
	} else {
		slog.Info("last snapshot ejected — resuming live OVN connection and reloading tables")
		if err := m.live.ResumeMonitors(ctx); err != nil {
			slog.Error("resuming live OVN monitors failed", "err", err)
			return
		}
	}
	m.liveSuspended = wantSuspended
}

// Close unloads every loaded snapshot and flushes any pending drains
// synchronously. Call it on shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.loaded {
		for _, fn := range e.stop {
			fn()
		}
		for _, fn := range e.drain {
			fn()
		}
		delete(m.loaded, id)
	}
	for seq, tail := range m.pendingDrains {
		tail()
		delete(m.pendingDrains, seq)
	}
}

// snapshotLabel builds a human-readable cluster label for a loaded snapshot.
func snapshotLabel(meta history.SnapshotMeta) string {
	ts := meta.Timestamp.Local().Format("2006-01-02 15:04")
	if meta.Label != "" {
		return fmt.Sprintf("Snapshot: %s (%s)", meta.Label, ts)
	}
	return fmt.Sprintf("Snapshot #%d (%s)", meta.ID, ts)
}
