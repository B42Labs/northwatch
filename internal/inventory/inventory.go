// Package inventory builds an aggregated, chassis-centric view of an OVN
// deployment entirely from the existing Southbound (SB) libovsdb cache. It
// introduces no new OVSDB connections.
//
// One entry joins five SB tables per chassis:
//
//   - Chassis: identity (name == OVS external_ids:system-id), hostname, and the
//     config copies ovn-controller mirrors into other_config (bridge mappings,
//     datapath-type, iface-types, ovn-cms-options).
//   - Encap: the chassis tunnel endpoint(s) (geneve/vxlan/stt, ip).
//   - Chassis_Private + SB_Global: liveness derived from nb_cfg sync. A chassis
//     is "alive" when it is present and has acknowledged the current
//     SB_Global.nb_cfg generation (in-sync); nb_cfg_timestamp is surfaced only
//     as informational age and to flag a lagging chassis as stale.
//   - Port_Binding: the logical ports bound to the chassis, summarized into a
//     per-chassis workload distribution.
//
// The data is config state mirrored by ovn-controller, not live OVS runtime
// state; surfacing real interface stats, flows, or link state requires a direct
// per-node Open_vSwitch connection (Phase 2). Chassis.name is the join key to
// that real OVS instance.
//
// The aggregation is a best-effort, eventually-consistent view: the chassis
// list and the per-table cache scans are independent reads that MonitorAll
// updates concurrently, so a config bump can momentarily surface a slightly
// inconsistent liveness or port count. A short-TTL result cache (see Builder)
// collapses repeated requests onto one computed snapshot, which both bounds the
// per-request cost of deep-copying the large SB tables and narrows this
// inconsistency window; a fully atomic snapshot would require a transactional
// read this code deliberately avoids.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// Other_config keys that ovn-controller mirrors from the local OVS instance.
const (
	keyBridgeMappings = "ovn-bridge-mappings"
)

// ErrNotFound is returned by Detail when no chassis carries the requested name.
var ErrNotFound = errors.New("chassis not found")

// EncapInfo is a single tunnel endpoint advertised by a chassis.
type EncapInfo struct {
	Type string `json:"type"`
	IP   string `json:"ip"`
}

// Liveness is the computed config-sync state of a chassis, derived from the
// Chassis_Private nb_cfg generation against SB_Global.nb_cfg. There is no
// boolean liveness column in the SB schema, so InSync, Alive and Stale are all
// computed here.
//
// A chassis with no Chassis_Private row reports InSync=false and Alive=false:
// ovn-controller creates and maintains that row, so its absence means there is
// no evidence of a controller acknowledging config for this chassis.
type Liveness struct {
	// InSync is true when the chassis has acknowledged the current SB_Global
	// nb_cfg generation (Chassis_Private.nb_cfg == SB_Global.nb_cfg).
	InSync bool `json:"in_sync"`
	// Alive is true when the chassis is present (has a Chassis_Private row) and
	// in-sync with the current nb_cfg generation. It deliberately does NOT key
	// off nb_cfg_timestamp age: that timestamp only advances when nb_cfg itself
	// changes, so on a steady-state cluster with no config churn it freezes and
	// an age-based check would report every healthy chassis down within seconds.
	Alive bool `json:"alive"`
	// Stale flags a chassis that is behind the current nb_cfg generation and has
	// not caught up within StaleThreshold — a genuinely lagging/stuck node. It is
	// an informational signal distinct from Alive: an in-sync chassis is never
	// stale, however old its last acknowledgement, so a quiet cluster stays clean.
	Stale bool `json:"stale"`
	// NbCfg is the chassis-acknowledged config generation (0 if no heartbeat).
	NbCfg int `json:"nb_cfg"`
	// SBNbCfg is the SB_Global config generation the chassis is compared against.
	SBNbCfg int `json:"sb_nb_cfg"`
	// NbCfgTimestamp is the chassis nb_cfg_timestamp in Unix milliseconds
	// (0 when absent).
	NbCfgTimestamp int64 `json:"nb_cfg_timestamp"`
	// AgeMs is the informational age of NbCfgTimestamp in milliseconds (0 when
	// absent, or when the timestamp is in the future relative to the northwatch
	// clock, which is clamped to 0 rather than leaking a negative age).
	AgeMs int64 `json:"age_ms"`
}

// PortSummary aggregates the Port_Bindings bound to a chassis.
type PortSummary struct {
	Total  int            `json:"total"`
	Up     int            `json:"up"`
	ByType map[string]int `json:"by_type"`
}

// BoundPort is a single logical port bound to a chassis.
type BoundPort struct {
	LogicalPort string `json:"logical_port"`
	Type        string `json:"type"`
	Up          *bool  `json:"up,omitempty"`
	TunnelKey   int    `json:"tunnel_key"`
}

// ChassisSummary is one entry in the aggregated chassis inventory list.
type ChassisSummary struct {
	// Name is the chassis identity (== Open_vSwitch external_ids:system-id) and
	// the join key a future Phase 2 uses to correlate SB chassis to per-node OVS.
	Name           string      `json:"name"`
	Hostname       string      `json:"hostname"`
	Encaps         []EncapInfo `json:"encaps"`
	BridgeMappings string      `json:"bridge_mappings,omitempty"`
	Liveness       Liveness    `json:"liveness"`
	Ports          PortSummary `json:"ports"`
}

// ChassisDetail extends ChassisSummary with the config copies and the full list
// of bound logical ports.
type ChassisDetail struct {
	ChassisSummary
	OtherConfig map[string]string `json:"other_config,omitempty"`
	BoundPorts  []BoundPort       `json:"bound_ports"`
}

// Builder aggregates the chassis inventory from the SB cache. It never opens a
// new connection: every method reads the in-memory libovsdb cache via SB.
//
// Builder holds a short-TTL cache of the last computed list and therefore must
// be used as a pointer (never copied) and may be shared across requests.
type Builder struct {
	SB client.Client
	// StaleThreshold is the maximum age of a chassis nb_cfg_timestamp before the
	// chassis is reported not-alive.
	StaleThreshold time.Duration
	// Now returns the current time; it is a seam for tests. nil falls back to
	// time.Now.
	Now func() time.Time

	// mu guards the short-TTL result cache below. List holds it across the whole
	// recompute so concurrent callers collapse onto a single scan (single-flight)
	// and reuse the cached snapshot instead of each deep-copying the SB tables.
	mu       sync.Mutex
	cachedAt time.Time
	cached   []ChassisSummary
}

// listCacheTTL bounds how long List reuses a previously computed inventory
// before recomputing. It is intentionally short: just long enough to collapse a
// burst of requests (e.g. a polling dashboard) onto one scan of the large SB
// tables while keeping the view close to live.
const listCacheTTL = 2 * time.Second

// caches holds the SB lookups gathered once per List/Detail call so liveness and
// port summaries are computed without repeated cache scans.
type caches struct {
	sbNbCfg        int
	privates       map[string]sb.ChassisPrivate // by Chassis_Private.name
	encaps         map[string]sb.Encap          // by Encap UUID
	portsByChassis map[string][]sb.PortBinding  // by Port_Binding.chassis UUID
}

// List returns one ChassisSummary per chassis, sorted by name. Results are
// served from a short-TTL cache (listCacheTTL): within the window the previously
// computed snapshot is returned unchanged, so the view is best-effort and
// eventually consistent rather than instantaneous. The returned slice is shared
// with the cache and must not be mutated by callers.
func (b *Builder) List(ctx context.Context) ([]ChassisSummary, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cached != nil && b.now().Sub(b.cachedAt) < listCacheTTL {
		return b.cached, nil
	}

	var chassisList []sb.Chassis
	if err := b.SB.List(ctx, &chassisList); err != nil {
		return nil, fmt.Errorf("listing chassis: %w", err)
	}

	c, err := b.loadCaches(ctx)
	if err != nil {
		return nil, err
	}

	summaries := make([]ChassisSummary, 0, len(chassisList))
	for _, ch := range chassisList {
		summaries = append(summaries, b.summarize(ch, c))
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })

	b.cached = summaries
	b.cachedAt = b.now()
	return summaries, nil
}

// Detail returns the full inventory entry for the chassis with the given name
// (== system-id). It returns ErrNotFound when no chassis carries that name.
func (b *Builder) Detail(ctx context.Context, name string) (*ChassisDetail, error) {
	var matches []sb.Chassis
	if err := b.SB.WhereCache(func(ch *sb.Chassis) bool {
		return ch.Name == name
	}).List(ctx, &matches); err != nil {
		return nil, fmt.Errorf("listing chassis %q: %w", name, err)
	}
	if len(matches) == 0 {
		return nil, ErrNotFound
	}
	ch := matches[0]

	c, err := b.loadCaches(ctx)
	if err != nil {
		return nil, err
	}

	return &ChassisDetail{
		ChassisSummary: b.summarize(ch, c),
		OtherConfig:    ch.OtherConfig,
		BoundPorts:     boundPorts(c.portsByChassis[ch.UUID]),
	}, nil
}

// summarize joins a single chassis with the shared caches into a ChassisSummary.
func (b *Builder) summarize(ch sb.Chassis, c *caches) ChassisSummary {
	s := ChassisSummary{
		Name:           ch.Name,
		Hostname:       ch.Hostname,
		BridgeMappings: ch.OtherConfig[keyBridgeMappings],
		Liveness:       b.computeLiveness(ch, c),
		Ports:          summarizePorts(c.portsByChassis[ch.UUID]),
	}
	for _, encapUUID := range ch.Encaps {
		if enc, ok := c.encaps[encapUUID]; ok {
			s.Encaps = append(s.Encaps, EncapInfo{Type: enc.Type, IP: enc.IP})
		}
	}
	sort.Slice(s.Encaps, func(i, j int) bool {
		if s.Encaps[i].Type != s.Encaps[j].Type {
			return s.Encaps[i].Type < s.Encaps[j].Type
		}
		return s.Encaps[i].IP < s.Encaps[j].IP
	})
	return s
}

// computeLiveness derives in-sync, alive and stale from the nb_cfg generation. A
// missing Chassis_Private row yields a zero-value Liveness (not in-sync, not
// alive), except SBNbCfg, which is always populated for context.
func (b *Builder) computeLiveness(ch sb.Chassis, c *caches) Liveness {
	l := Liveness{SBNbCfg: c.sbNbCfg}
	priv, found := c.privates[ch.Name]
	if !found {
		return l
	}
	l.NbCfg = priv.NbCfg
	l.NbCfgTimestamp = int64(priv.NbCfgTimestamp)
	l.InSync = priv.NbCfg == c.sbNbCfg
	// A present-and-in-sync chassis is alive. We do NOT key alive off the age of
	// nb_cfg_timestamp: that timestamp only advances when nb_cfg itself changes
	// (see ovn-sb(5), Chassis_Private:nb_cfg_timestamp), so on a steady-state
	// cluster with no config churn it freezes and an age check would mark every
	// healthy chassis down once the staleness window elapses.
	l.Alive = l.InSync
	if priv.NbCfgTimestamp > 0 {
		// AgeMs is informational only. nb_cfg_timestamp is written by the
		// chassis's own ovn-controller clock, foreign relative to b.now(); a
		// future timestamp (age < 0) means clock skew and is clamped to 0 rather
		// than leaking a negative age into the response.
		if age := b.now().UnixMilli() - int64(priv.NbCfgTimestamp); age > 0 {
			l.AgeMs = age
		}
		// Stale is the age-based signal, scoped to an out-of-sync chassis: it has
		// received a newer nb_cfg generation but has not acknowledged it within
		// StaleThreshold, i.e. it is lagging/stuck rather than merely mid-update.
		l.Stale = !l.InSync && l.AgeMs > b.StaleThreshold.Milliseconds()
	}
	return l
}

// loadCaches gathers the SB lookups used to compute liveness and port summaries.
func (b *Builder) loadCaches(ctx context.Context) (*caches, error) {
	c := &caches{
		privates:       map[string]sb.ChassisPrivate{},
		encaps:         map[string]sb.Encap{},
		portsByChassis: map[string][]sb.PortBinding{},
	}

	var globals []sb.SBGlobal
	if err := b.SB.List(ctx, &globals); err != nil {
		return nil, fmt.Errorf("listing sb_global: %w", err)
	}
	if len(globals) > 0 {
		c.sbNbCfg = globals[0].NbCfg
	}

	var privates []sb.ChassisPrivate
	if err := b.SB.List(ctx, &privates); err != nil {
		return nil, fmt.Errorf("listing chassis_private: %w", err)
	}
	for _, p := range privates {
		c.privates[p.Name] = p
	}

	var encaps []sb.Encap
	if err := b.SB.List(ctx, &encaps); err != nil {
		return nil, fmt.Errorf("listing encaps: %w", err)
	}
	for _, e := range encaps {
		c.encaps[e.UUID] = e
	}

	var bindings []sb.PortBinding
	if err := b.SB.List(ctx, &bindings); err != nil {
		return nil, fmt.Errorf("listing port bindings: %w", err)
	}
	for _, pb := range bindings {
		if pb.Chassis != nil && *pb.Chassis != "" {
			c.portsByChassis[*pb.Chassis] = append(c.portsByChassis[*pb.Chassis], pb)
		}
	}

	return c, nil
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// summarizePorts counts bound ports by liveness and type.
func summarizePorts(pbs []sb.PortBinding) PortSummary {
	s := PortSummary{ByType: map[string]int{}}
	for _, pb := range pbs {
		s.Total++
		if pb.Up != nil && *pb.Up {
			s.Up++
		}
		s.ByType[pb.Type]++
	}
	return s
}

// boundPorts lists the bound ports sorted by logical port name.
func boundPorts(pbs []sb.PortBinding) []BoundPort {
	out := make([]BoundPort, 0, len(pbs))
	for _, pb := range pbs {
		out = append(out, BoundPort{
			LogicalPort: pb.LogicalPort,
			Type:        pb.Type,
			Up:          pb.Up,
			TunnelKey:   pb.TunnelKey,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LogicalPort < out[j].LogicalPort })
	return out
}
