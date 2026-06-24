// Package snapshotsession loads stored history snapshots as live, switchable
// data sources at runtime. A loaded snapshot is served by in-memory OVSDB
// servers and registered as an additional read-only cluster, so the UI can
// switch to it (and back) without restarting Northwatch.
package snapshotsession

import (
	"context"
	"fmt"
	"log"
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

	mu     sync.Mutex
	loaded map[int64]*entry
}

type entry struct {
	loaded  Loaded
	cleanup []func()
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
		store:      store,
		reg:        reg,
		nbModel:    nbModel,
		sbModel:    sbModel,
		nbSchema:   nbSchema,
		sbSchema:   sbSchema,
		build:      build,
		register:   register,
		unregister: unregister,
		live:       live,
		loaded:     make(map[int64]*entry),
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
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.loaded[id]; ok {
		return e.loaded, nil
	}

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

	m.reg.Register(name, c)
	m.register(c)

	ld := Loaded{Cluster: name, Label: label, Mode: "snapshot", SourceID: id, CreatedAt: createdAt}

	// Cleanup runs in order on unload: stop routing, drop from the registry, stop
	// any background work, close the clients, then the in-memory servers.
	cleanup := []func(){
		func() { m.unregister(name) },
		func() { m.reg.Unregister(name) },
	}
	cleanup = append(cleanup, stops...)
	cleanup = append(cleanup, c.DBs.Close, servers.Close)

	// First snapshot loaded: suspend the live OVN connection so it stops
	// streaming updates while the operator works on the snapshot.
	firstLoad := len(m.loaded) == 0
	m.loaded[id] = &entry{loaded: ld, cleanup: cleanup}
	if firstLoad && m.live != nil {
		log.Printf("snapshot: %s loaded — suspending live OVN connection", name)
		if err := m.live.SuspendMonitors(ctx); err != nil {
			log.Printf("snapshot: suspending live OVN monitors failed: %v", err)
		}
	}

	return ld, nil
}

// Unload removes a loaded snapshot cluster and frees its resources. When the
// last loaded snapshot is removed, the live OVN connection is resumed and its
// tables reloaded.
func (m *Manager) Unload(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.loaded[id]
	if !ok {
		return ErrNotLoaded
	}
	for _, fn := range e.cleanup {
		fn()
	}
	delete(m.loaded, id)

	if len(m.loaded) == 0 && m.live != nil {
		log.Printf("snapshot: %s ejected — resuming live OVN connection and reloading tables", clusterName(id))
		if err := m.live.ResumeMonitors(ctx); err != nil {
			log.Printf("snapshot: resuming live OVN monitors failed: %v", err)
		}
	}
	return nil
}

// Close unloads every loaded snapshot. Call it on shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.loaded {
		for _, fn := range e.cleanup {
			fn()
		}
		delete(m.loaded, id)
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
