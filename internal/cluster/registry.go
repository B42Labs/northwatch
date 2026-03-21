package cluster

import (
	"sync"

	"github.com/b42labs/northwatch/internal/alert"
	"github.com/b42labs/northwatch/internal/correlate"
	"github.com/b42labs/northwatch/internal/debug"
	"github.com/b42labs/northwatch/internal/enrich"
	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/flowdiff"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/search"
	"github.com/b42labs/northwatch/internal/telemetry"
)

// Cluster holds all subsystems for a single OVN deployment.
type Cluster struct {
	Name                 string
	Label                string
	DBs                  *ovndb.OVNDatabases
	Correlator           *correlate.Correlator
	Enricher             *enrich.Enricher
	EventHub             *events.Hub
	SearchEngine         *search.Engine
	FlowDiff             *flowdiff.Store
	AlertEngine          *alert.Engine
	Telemetry            *telemetry.Querier
	ConnectivityChecker  *debug.ConnectivityChecker
	PortDiagnoser        *debug.PortDiagnoser
	ACLAuditor           *debug.ACLAuditor
	StaleDetector        *debug.StaleDetector
}

// Registry manages multiple named clusters.
type Registry struct {
	mu       sync.RWMutex
	clusters map[string]*Cluster
	order    []string // insertion order
}

// NewRegistry creates a new empty cluster registry.
func NewRegistry() *Registry {
	return &Registry{clusters: make(map[string]*Cluster)}
}

// Register adds a named cluster to the registry.
func (r *Registry) Register(name string, c *Cluster) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.clusters[name]; !exists {
		r.order = append(r.order, name)
	}
	r.clusters[name] = c
}

// Get returns a cluster by name.
func (r *Registry) Get(name string) (*Cluster, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clusters[name]
	return c, ok
}

// Default returns the first registered cluster, or nil if empty.
func (r *Registry) Default() *Cluster {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.order) == 0 {
		return nil
	}
	return r.clusters[r.order[0]]
}

// List returns all clusters in registration order.
func (r *Registry) List() []*Cluster {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Cluster, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.clusters[name])
	}
	return result
}

// Len returns the number of registered clusters.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clusters)
}

// Close shuts down all cluster database connections.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.clusters {
		c.DBs.Close()
	}
}
