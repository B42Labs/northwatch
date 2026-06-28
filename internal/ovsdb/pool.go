package ovsdb

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
)

// ovsConnectTimeout bounds a single Connect+MonitorAll attempt against one
// chassis so a black-holed management address does not hang the supervisor.
const ovsConnectTimeout = 10 * time.Second

// ovsMember is one chassis connection in the pool.
type ovsMember struct {
	systemID string
	addr     string
	client   client.Client
}

// OVSMemberStatus is the read-only status of one chassis connection, returned by
// OVSPool.Members for the fleet-status endpoint.
type OVSMemberStatus struct {
	SystemID  string `json:"system_id"`
	Connected bool   `json:"connected"`
}

// OVSPool is a dynamic pool of per-chassis Open_vSwitch OVSDB connections. Each
// member maintains its own libovsdb client and TableCache, monitored exactly
// like the NB/SB connections. The pool is the N-connection generalization the
// OVS visibility integration needs: members connect independently and a node
// that is unreachable (or never enables remote OVSDB access) keeps retrying in
// its own goroutine without affecting the other members or the NB/SB views.
type OVSPool struct {
	model     model.ClientDBModel
	tlsConfig *tls.Config

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.RWMutex
	members map[string]*ovsMember
	order   []string
}

// NewOVSPool creates an empty pool. dbModel must be vs.FullDatabaseModel();
// tlsConfig is nil for plain tcp: endpoints and non-nil for ssl: endpoints.
func NewOVSPool(dbModel model.ClientDBModel, tlsConfig *tls.Config) *OVSPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &OVSPool{
		model:     dbModel,
		tlsConfig: tlsConfig,
		ctx:       ctx,
		cancel:    cancel,
		members:   make(map[string]*ovsMember),
	}
}

// Add registers a chassis by system-id and management address and launches a
// supervised goroutine that connects and monitors it. addr may be a
// comma-separated list for failover. Adding a system-id that already exists is
// an error. The call returns as soon as the client is created; the connection
// is established in the background so one slow or unreachable chassis never
// blocks the others.
func (p *OVSPool) Add(systemID, addr string) error {
	opts := append(splitEndpoints(addr), client.WithReconnect(ovsConnectTimeout, newBackoff()))
	if p.tlsConfig != nil {
		opts = append(opts, client.WithTLSConfig(p.tlsConfig))
	}
	c, err := client.NewOVSDBClient(p.model, opts...)
	if err != nil {
		return fmt.Errorf("creating OVS client for %s: %w", systemID, err)
	}

	m := &ovsMember{systemID: systemID, addr: addr, client: c}

	p.mu.Lock()
	if _, exists := p.members[systemID]; exists {
		p.mu.Unlock()
		c.Close()
		return fmt.Errorf("OVS chassis %q already registered", systemID)
	}
	p.members[systemID] = m
	p.order = append(p.order, systemID)
	p.mu.Unlock()

	p.wg.Add(1)
	go p.supervise(m)
	return nil
}

// supervise repeatedly attempts to connect and monitor a single member until it
// succeeds or the pool is closed. After the first successful monitor, libovsdb's
// WithReconnect keeps the connection alive across drops, so the supervisor's job
// is only to get the very first connection up — retrying with capped backoff for
// a chassis that has not yet enabled remote OVSDB access or is briefly down.
func (p *OVSPool) supervise(m *ovsMember) {
	defer p.wg.Done()

	bo := newBackoff()
	for {
		if err := p.connectAndMonitor(m); err != nil {
			log.Printf("ovs: chassis %s (%s) unreachable, retrying: %v", m.systemID, m.addr, err)
			select {
			case <-p.ctx.Done():
				return
			case <-time.After(bo.NextBackOff()):
				continue
			}
		}
		log.Printf("ovs: chassis %s (%s) connected and monitored", m.systemID, m.addr)
		return
	}
}

// connectAndMonitor performs one bounded Connect+MonitorAll attempt.
func (p *OVSPool) connectAndMonitor(m *ovsMember) error {
	ctx, cancel := context.WithTimeout(p.ctx, ovsConnectTimeout)
	defer cancel()

	if err := m.client.Connect(ctx); err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	if _, err := m.client.MonitorAll(ctx); err != nil {
		return fmt.Errorf("monitoring: %w", err)
	}
	return nil
}

// Client returns the libovsdb client for a chassis by system-id. The bool is
// false when no such chassis is registered. The returned client may not be
// Connected yet (or may have dropped) — callers check c.Connected().
func (p *OVSPool) Client(systemID string) (client.Client, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	m, ok := p.members[systemID]
	if !ok {
		return nil, false
	}
	return m.client, true
}

// Members returns the status of every registered chassis in registration order.
func (p *OVSPool) Members() []OVSMemberStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]OVSMemberStatus, 0, len(p.order))
	for _, id := range p.order {
		m := p.members[id]
		out = append(out, OVSMemberStatus{
			SystemID:  m.systemID,
			Connected: m.client.Connected(),
		})
	}
	return out
}

// Close cancels every supervisor goroutine, waits for them to exit, and closes
// all member clients.
func (p *OVSPool) Close() {
	p.cancel()
	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range p.order {
		p.members[id].client.Close()
	}
}

// ConnectOVSPool builds an OVSPool from a system-id → mgmt-addr mapping and
// registers every entry. Per-member registration failures are logged and
// skipped (non-fatal) so one bad address does not prevent the rest of the fleet
// from being monitored. Returns the pool even when some members failed to
// register; an empty mapping yields an empty pool.
func ConnectOVSPool(dbModel model.ClientDBModel, tlsConfig *tls.Config, addrs map[string]string) *OVSPool {
	pool := NewOVSPool(dbModel, tlsConfig)
	for systemID, addr := range addrs {
		if err := pool.Add(systemID, addr); err != nil {
			log.Printf("ovs: skipping chassis %s: %v", systemID, err)
		}
	}
	return pool
}
