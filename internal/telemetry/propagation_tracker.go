package telemetry

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// PropagationTracker subscribes to the event hub and records
// when each chassis catches up to each NB config generation.
type PropagationTracker struct {
	hub   *events.Hub
	store *PropagationStore
	nb    client.Client
	sb    client.Client

	mu             sync.Mutex
	currentGen     int              // latest NB_Global.nb_cfg
	currentNbTsMs  int64            // latest NB_Global.nb_cfg_timestamp
	chassisLastGen map[string]int   // per-chassis: last recorded generation
	hostnames      map[string]string // chassis name -> hostname
}

// NewPropagationTracker creates a new tracker.
func NewPropagationTracker(hub *events.Hub, store *PropagationStore, nbClient, sbClient client.Client) *PropagationTracker {
	return &PropagationTracker{
		hub:            hub,
		store:          store,
		nb:             nbClient,
		sb:             sbClient,
		chassisLastGen: make(map[string]int),
		hostnames:      make(map[string]string),
	}
}

// Start seeds the tracker state and begins listening for events.
// Returns a cleanup function.
func (t *PropagationTracker) Start(ctx context.Context) func() {
	t.seed(ctx)

	sub := t.hub.Subscribe()
	sub.AddFilter(events.Filter{Database: "nb", Tables: []string{"NB_Global"}})
	sub.AddFilter(events.Filter{Database: "sb", Tables: []string{"Chassis_Private", "Chassis"}})

	done := make(chan struct{})
	go func() {
		for {
			select {
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				t.handleEvent(ctx, evt)
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
		t.hub.Unsubscribe(sub)
	}
}

// seed reads current state from OVSDB to initialize the tracker.
func (t *PropagationTracker) seed(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Read current NB_Global
	var nbGlobals []nb.NBGlobal
	if err := t.nb.List(ctx, &nbGlobals); err == nil && len(nbGlobals) > 0 {
		t.currentGen = nbGlobals[0].NbCfg
		t.currentNbTsMs = int64(nbGlobals[0].NbCfgTimestamp)
	}

	// Read chassis hostnames
	var chassisList []sb.Chassis
	if err := t.sb.List(ctx, &chassisList); err == nil {
		for _, ch := range chassisList {
			t.hostnames[ch.Name] = ch.Hostname
		}
	}

	// Read Chassis_Private to seed chassisLastGen
	var privates []sb.ChassisPrivate
	if err := t.sb.List(ctx, &privates); err == nil {
		for _, p := range privates {
			t.chassisLastGen[p.Name] = p.NbCfg
		}
	}
}

func (t *PropagationTracker) handleEvent(ctx context.Context, evt events.Event) {
	switch {
	case evt.Database == "nb" && evt.Table == "NB_Global":
		t.handleNBGlobalUpdate(ctx, evt)
	case evt.Database == "sb" && evt.Table == "Chassis_Private":
		t.handleChassisPrivateUpdate(evt)
	case evt.Database == "sb" && evt.Table == "Chassis":
		t.handleChassisEvent(evt)
	}
}

func (t *PropagationTracker) handleNBGlobalUpdate(ctx context.Context, evt events.Event) {
	newCfg := intFromRow(evt.Row, "nb_cfg")
	newTs := int64FromRow(evt.Row, "nb_cfg_timestamp")

	t.mu.Lock()
	defer t.mu.Unlock()

	if newCfg > t.currentGen {
		t.currentGen = newCfg
		// nb_cfg_timestamp may still reflect the previous generation because
		// the CMS increments nb_cfg and ovn-northd writes nb_cfg_timestamp
		// in separate transactions. Only accept a timestamp that is newer
		// than the one we already have; otherwise wait for the real one.
		if newTs > t.currentNbTsMs {
			t.currentNbTsMs = newTs
		} else {
			t.currentNbTsMs = 0
		}
	} else if newCfg == t.currentGen && newTs > 0 && (t.currentNbTsMs == 0 || newTs > t.currentNbTsMs) {
		// ovn-northd updated nb_cfg_timestamp for the current generation.
		t.currentNbTsMs = newTs
	} else {
		return
	}

	if t.currentNbTsMs <= 0 {
		return
	}

	// Check which chassis are already caught up to the new generation.
	// sb.List reads from libovsdb's in-memory cache, so it's safe to hold the lock.
	var privates []sb.ChassisPrivate
	if err := t.sb.List(ctx, &privates); err != nil {
		log.Printf("propagation-tracker: listing Chassis_Private: %v", err)
		return
	}

	for _, p := range privates {
		if p.NbCfg >= t.currentGen && t.chassisLastGen[p.Name] < t.currentGen {
			t.chassisLastGen[p.Name] = p.NbCfg
			if int64(p.NbCfgTimestamp) > 0 {
				latency := int64(p.NbCfgTimestamp) - t.currentNbTsMs
				if latency < 0 {
					latency = 0
				}
				t.store.Add(PropagationEvent{
					Generation:         t.currentGen,
					NbTimestampMs:      t.currentNbTsMs,
					Chassis:            p.Name,
					Hostname:           t.hostnames[p.Name],
					ChassisTimestampMs: int64(p.NbCfgTimestamp),
					LatencyMs:          latency,
					RecordedAt:         time.Now().UnixMilli(),
				})
			}
		}
	}
}

func (t *PropagationTracker) handleChassisPrivateUpdate(evt events.Event) {
	name := stringFromRow(evt.Row, "name")
	cfg := intFromRow(evt.Row, "nb_cfg")
	cfgTs := int64FromRow(evt.Row, "nb_cfg_timestamp")

	if name == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	gen := t.currentGen
	nbTs := t.currentNbTsMs

	if gen == 0 || cfg < gen {
		return
	}
	if t.chassisLastGen[name] >= gen {
		return
	}
	// Don't record until we have the confirmed NB timestamp for this
	// generation. The chassis-scan in handleNBGlobalUpdate will pick
	// this chassis up once the real timestamp arrives.
	if nbTs <= 0 {
		return
	}

	t.chassisLastGen[name] = cfg
	if cfgTs > 0 {
		latency := cfgTs - nbTs
		if latency < 0 {
			latency = 0
		}
		t.store.Add(PropagationEvent{
			Generation:         gen,
			NbTimestampMs:      nbTs,
			Chassis:            name,
			Hostname:           t.hostnames[name],
			ChassisTimestampMs: cfgTs,
			LatencyMs:          latency,
			RecordedAt:         time.Now().UnixMilli(),
		})
	}
}

func (t *PropagationTracker) handleChassisEvent(evt events.Event) {
	name := stringFromRow(evt.Row, "name")
	if name == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch evt.Type {
	case events.EventInsert, events.EventUpdate:
		hostname := stringFromRow(evt.Row, "hostname")
		t.hostnames[name] = hostname
	case events.EventDelete:
		delete(t.hostnames, name)
		delete(t.chassisLastGen, name)
	}
}

// Helper functions to extract typed values from event row maps.

func intFromRow(row map[string]any, key string) int {
	if row == nil {
		return 0
	}
	v, ok := row[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	}
	return 0
}

func int64FromRow(row map[string]any, key string) int64 {
	if row == nil {
		return 0
	}
	v, ok := row[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

func stringFromRow(row map[string]any, key string) string {
	if row == nil {
		return ""
	}
	v, ok := row[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
