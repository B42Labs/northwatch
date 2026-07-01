package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/api"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/b42labs/northwatch/internal/ovshealth"
)

// ovsTableAccess holds the list/get closures for one whitelisted OVS table,
// generic over the table's model type but called with a per-chassis client.
type ovsTableAccess struct {
	list func(ctx context.Context, c client.Client) ([]map[string]any, error)
	get  func(ctx context.Context, c client.Client, uuid string) (map[string]any, bool, error)
}

// ovsAccess builds the access closures for an OVS model type, reusing the same
// cache queries (List / WhereCache) and JSON mapping as the NB/SB handlers.
func ovsAccess[T any]() ovsTableAccess {
	return ovsTableAccess{
		list: func(ctx context.Context, c client.Client) ([]map[string]any, error) {
			var results []T
			if err := c.List(ctx, &results); err != nil {
				return nil, err
			}
			return api.ModelsToMaps(results), nil
		},
		get: func(ctx context.Context, c client.Client, uuid string) (map[string]any, bool, error) {
			var results []T
			if err := c.WhereCache(func(m *T) bool {
				return getUUID(m) == uuid
			}).List(ctx, &results); err != nil {
				return nil, false, err
			}
			if len(results) == 0 {
				return nil, false, nil
			}
			return api.ModelToMap(results[0]), true, nil
		},
	}
}

// ovsAccessRedacted builds access closures for an OVS model type like ovsAccess,
// but deletes the named sensitive keys from every returned map before it reaches
// the client (e.g. SSL.private_key). The redacted keys are simply absent from
// the JSON rather than blanked, so the generic renderer never displays them.
func ovsAccessRedacted[T any](redact ...string) ovsTableAccess {
	base := ovsAccess[T]()
	return ovsTableAccess{
		list: func(ctx context.Context, c client.Client) ([]map[string]any, error) {
			rows, err := base.list(ctx, c)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				for _, k := range redact {
					delete(row, k)
				}
			}
			return rows, nil
		},
		get: func(ctx context.Context, c client.Client, uuid string) (map[string]any, bool, error) {
			row, found, err := base.get(ctx, c, uuid)
			if err != nil || !found {
				return row, found, err
			}
			for _, k := range redact {
				delete(row, k)
			}
			return row, true, nil
		},
	}
}

// ovsTables whitelists the read-only per-chassis Open_vSwitch tables the
// integration exposes — all the vswitchd tables the monitored cache already
// holds. The keys are the URL path segments (the OVSDB table name lowercased
// with "_" → "-"); only these are routable so an arbitrary OVS table cannot be
// reached. The ssl row redacts private_key so SSL key material is never served;
// certificate and ca_cert are public X.509 material and stay.
var ovsTables = map[string]ovsTableAccess{
	"interface":    ovsAccess[vs.Interface](),
	"bridge":       ovsAccess[vs.Bridge](),
	"port":         ovsAccess[vs.Port](),
	"open-vswitch": ovsAccess[vs.OpenvSwitch](),
	"manager":      ovsAccess[vs.Manager](),
	"controller":   ovsAccess[vs.Controller](),

	// Monitoring / export-config tables.
	"ipfix":   ovsAccess[vs.IPFIX](),
	"sflow":   ovsAccess[vs.SFlow](),
	"netflow": ovsAccess[vs.NetFlow](),
	"mirror":  ovsAccess[vs.Mirror](),
	"qos":     ovsAccess[vs.QoS](),
	"queue":   ovsAccess[vs.Queue](),

	// Conntrack tables.
	"ct-zone":           ovsAccess[vs.CTZone](),
	"ct-timeout-policy": ovsAccess[vs.CTTimeoutPolicy](),
	"datapath":          ovsAccess[vs.Datapath](),

	// Remaining tables.
	"flow-table":                ovsAccess[vs.FlowTable](),
	"flow-sample-collector-set": ovsAccess[vs.FlowSampleCollectorSet](),
	"ssl":                       ovsAccessRedacted[vs.SSL]("private_key"),
	"autoattach":                ovsAccess[vs.AutoAttach](),
}

// ovsPool is the read-only view of the chassis connection pool the fleet-health
// aggregation needs: the fleet membership and a client lookup by system-id.
// *ovndb.OVSPool satisfies it; tests substitute a fake to inject a chassis whose
// cache read fails.
type ovsPool interface {
	Members() []ovndb.OVSMemberStatus
	Client(systemID string) (client.Client, bool)
}

// ovsHealthTTL bounds how long a computed fleet-health snapshot is served before
// it is recomputed. It coalesces bursts of /api/v1/ovs/health polls into at most
// one fleet-wide cache fan-out per interval, so a short-polling dashboard (or an
// unthrottled caller) cannot force a full Bridge/Port/Interface materialization
// across every chassis on every request.
const ovsHealthTTL = time.Second

// healthCache memoizes the most recent fleet-health snapshot for ovsHealthTTL.
// The mutex is held across the recompute so a burst of concurrent requests
// coalesces onto a single fan-out rather than each triggering its own.
type healthCache struct {
	mu  sync.Mutex
	at  time.Time
	val ovshealth.FleetHealth
}

// get returns the cached fleet health when it is younger than ovsHealthTTL,
// otherwise it recomputes, stores and returns a fresh snapshot. now is passed in
// so tests can drive the TTL deterministically.
func (hc *healthCache) get(ctx context.Context, pool ovsPool, now time.Time) ovshealth.FleetHealth {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if !hc.at.IsZero() && now.Sub(hc.at) < ovsHealthTTL {
		return hc.val
	}
	hc.val = ovsFleetHealth(ctx, pool)
	hc.at = now
	return hc.val
}

// RegisterOVS registers the read-only per-chassis Open_vSwitch endpoints,
// addressed by system-id (the SB Chassis.name / OVS external_ids:system-id join
// key). GET /api/v1/ovs reports the fleet connection status; the per-chassis
// table routes serve live OVS state from that chassis's monitored cache.
func RegisterOVS(mux *http.ServeMux, pool *ovndb.OVSPool) {
	mux.HandleFunc("GET /api/v1/ovs", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, pool.Members())
	})

	var health healthCache
	mux.HandleFunc("GET /api/v1/ovs/health", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, health.get(r.Context(), pool, time.Now()))
	})

	mux.HandleFunc("GET /api/v1/ovs/{chassis}/{table}", func(w http.ResponseWriter, r *http.Request) {
		c, access, ok := resolveOVS(w, r, pool)
		if !ok {
			return
		}
		rows, err := access.list(r.Context(), c)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		api.WriteJSON(w, http.StatusOK, rows)
	})

	mux.HandleFunc("GET /api/v1/ovs/{chassis}/{table}/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		c, access, ok := resolveOVS(w, r, pool)
		if !ok {
			return
		}
		uuid := r.PathValue("uuid")
		if uuid == "" {
			api.WriteError(w, http.StatusBadRequest, "uuid is required")
			return
		}
		row, found, err := access.get(r.Context(), c, uuid)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !found {
			api.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		api.WriteJSON(w, http.StatusOK, row)
	})
}

// ovsFleetHealth reads the live bridges, ports and interfaces from every
// connected chassis's monitored cache and aggregates them into a fleet-wide
// health summary. An unreachable chassis is listed but its cache is not read, so
// it is excluded from the totals rather than counted as healthy. A connected
// chassis whose cache read fails is treated the same way — logged and excluded —
// so one node's failure degrades that node rather than blanking the whole fleet.
func ovsFleetHealth(ctx context.Context, pool ovsPool) ovshealth.FleetHealth {
	members := pool.Members()
	caches := make([]ovshealth.ChassisCache, 0, len(members))
	for _, m := range members {
		cc := ovshealth.ChassisCache{SystemID: m.SystemID}
		// Only read a connected chassis. Client is always present for a
		// registered member (the pool has no removal path); the ok guard keeps
		// the aggregation correct if that ever changes by treating a vanished
		// member as unreachable.
		if c, ok := pool.Client(m.SystemID); m.Connected && ok {
			if err := listChassisCache(ctx, c, &cc); err != nil {
				// Exclude just this chassis: reset its partially-filled cache so
				// it aggregates as unreachable (connected=false, zero counts).
				log.Printf("ovs: health: excluding chassis %s: %v", m.SystemID, err)
				cc = ovshealth.ChassisCache{SystemID: m.SystemID}
			} else {
				cc.Connected = true
			}
		}
		caches = append(caches, cc)
	}
	return ovshealth.Aggregate(caches)
}

// listChassisCache reads a connected chassis's bridges, ports and interfaces
// from its monitored cache into cc. The reads hit the in-memory cache and
// effectively never fail; a genuine read error is returned so the caller can
// exclude the chassis rather than fail the whole fleet.
func listChassisCache(ctx context.Context, c client.Client, cc *ovshealth.ChassisCache) error {
	if err := c.List(ctx, &cc.Bridges); err != nil {
		return fmt.Errorf("listing bridges: %w", err)
	}
	if err := c.List(ctx, &cc.Ports); err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}
	if err := c.List(ctx, &cc.Interfaces); err != nil {
		return fmt.Errorf("listing interfaces: %w", err)
	}
	return nil
}

// resolveOVSChassis resolves the {chassis} path value to a connected client. It
// writes the appropriate error (404 unknown chassis, 503 unreachable chassis)
// and returns ok=false when the request cannot be served. The correlation
// handler shares these reachability semantics with the table routes.
func resolveOVSChassis(w http.ResponseWriter, r *http.Request, pool *ovndb.OVSPool) (client.Client, bool) {
	chassis := r.PathValue("chassis")
	c, ok := pool.Client(chassis)
	if !ok {
		api.WriteError(w, http.StatusNotFound, "unknown chassis")
		return nil, false
	}
	if !c.Connected() {
		api.WriteError(w, http.StatusServiceUnavailable, "chassis unreachable")
		return nil, false
	}
	return c, true
}

// resolveOVS resolves the {chassis} and {table} path values to a connected
// client and the table's access closures. It writes the appropriate error
// (404 unknown chassis, 503 unreachable chassis, 404 unknown table) and returns
// ok=false when the request cannot be served.
func resolveOVS(w http.ResponseWriter, r *http.Request, pool *ovndb.OVSPool) (client.Client, ovsTableAccess, bool) {
	c, ok := resolveOVSChassis(w, r, pool)
	if !ok {
		return nil, ovsTableAccess{}, false
	}
	table := r.PathValue("table")
	access, ok := ovsTables[table]
	if !ok {
		api.WriteError(w, http.StatusNotFound, "unknown table")
		return nil, ovsTableAccess{}, false
	}
	return c, access, true
}
