package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/testutil"
)

// chassisPrivateEvent builds a synthetic Chassis_Private update event carrying
// the per-chassis config generation the tracker reads straight from the row.
func chassisPrivateEvent(name string, nbCfg int, nbCfgTs int64) events.Event {
	return events.Event{
		Type:     events.EventUpdate,
		Database: "sb",
		Table:    "Chassis_Private",
		UUID:     name,
		Row: map[string]any{
			"name":             name,
			"nb_cfg":           nbCfg,
			"nb_cfg_timestamp": nbCfgTs,
		},
		Ts: 0,
	}
}

// nbGlobalEvent builds a synthetic NB_Global update event.
func nbGlobalEvent(nbCfg int, nbCfgTs int64) events.Event {
	return events.Event{
		Type:     events.EventUpdate,
		Database: "nb",
		Table:    "NB_Global",
		Row: map[string]any{
			"nb_cfg":           nbCfg,
			"nb_cfg_timestamp": nbCfgTs,
		},
	}
}

// TestPropTracker_CatchUp drives the Chassis_Private catch-up path directly:
// after seeding at gen 5 with a chassis behind at gen 4, a catch-up event
// records one propagation sample with the measured latency.
func TestPropTracker_CatchUp(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 4, 9500)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(events.NewHub(), store, nbClient, sbClient)
	tracker.seed(context.Background())

	tracker.handleEvent(context.Background(), chassisPrivateEvent("ch-1", 5, 10300))

	evts := store.Query("ch-1", 0)
	require.Len(t, evts, 1)
	assert.Equal(t, 5, evts[0].Generation)
	assert.Equal(t, "ch-1", evts[0].Chassis)
	assert.Equal(t, "host-1", evts[0].Hostname)
	assert.Equal(t, int64(10000), evts[0].NbTimestampMs)
	assert.Equal(t, int64(10300), evts[0].ChassisTimestampMs)
	assert.Equal(t, int64(300), evts[0].LatencyMs)
}

// TestPropTracker_GenBump drives the NB_Global-scan path: when NB advances to a
// new generation, the tracker scans Chassis_Private from the SB cache and
// records any chassis already caught up.
func TestPropTracker_GenBump(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 5, 10200)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(events.NewHub(), store, nbClient, sbClient)
	tracker.seed(context.Background())

	// Chassis reaches gen 6 in the SB cache, then NB_Global bumps to gen 6.
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 6, 20500)
	tracker.handleEvent(context.Background(), nbGlobalEvent(6, 20000))

	evts := store.Query("ch-1", 0)
	require.Len(t, evts, 1)
	assert.Equal(t, 6, evts[0].Generation)
	assert.Equal(t, int64(500), evts[0].LatencyMs)
}

// TestPropTracker_NoDup asserts a repeated catch-up event at the same
// generation does not record a second sample.
func TestPropTracker_NoDup(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 4, 9500)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(events.NewHub(), store, nbClient, sbClient)
	tracker.seed(context.Background())

	tracker.handleEvent(context.Background(), chassisPrivateEvent("ch-1", 5, 10300))
	require.Len(t, store.Query("ch-1", 0), 1)

	tracker.handleEvent(context.Background(), chassisPrivateEvent("ch-1", 5, 10300))
	assert.Len(t, store.Query("ch-1", 0), 1)
}

// TestPropTracker_StaleTimestamp reproduces the real OVN flow where the CMS
// increments nb_cfg and ovn-northd writes nb_cfg_timestamp in separate
// transactions: nothing is recorded until the confirmed NB timestamp arrives.
func TestPropTracker_StaleTimestamp(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 5, 10200)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(events.NewHub(), store, nbClient, sbClient)
	tracker.seed(context.Background())

	// Step 1: CMS bumps gen to 6, but the timestamp is still from gen 5 (stale),
	// so the tracker parks the timestamp at zero and records nothing.
	tracker.handleEvent(context.Background(), nbGlobalEvent(6, 10000))
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 6, 20500)

	// Step 2: a chassis catch-up event while the NB timestamp is unknown is
	// held back, not recorded.
	tracker.handleEvent(context.Background(), chassisPrivateEvent("ch-1", 6, 20500))
	assert.Empty(t, store.Query("", 0), "should not record with stale NB timestamp")

	// Step 3: ovn-northd writes the real timestamp; the scan now records.
	tracker.handleEvent(context.Background(), nbGlobalEvent(6, 20000))

	evts := store.Query("ch-1", 0)
	require.Len(t, evts, 1)
	assert.Equal(t, 6, evts[0].Generation)
	assert.Equal(t, int64(20000), evts[0].NbTimestampMs)
	assert.Equal(t, int64(500), evts[0].LatencyMs)
}

// TestPropTracker_ZeroChassisTs asserts a chassis that reaches the current
// generation but has no timestamp yet is marked caught up without recording a
// bogus zero-latency sample.
func TestPropTracker_ZeroChassisTs(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 4, 0)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(events.NewHub(), store, nbClient, sbClient)
	tracker.seed(context.Background())

	tracker.handleEvent(context.Background(), chassisPrivateEvent("ch-1", 5, 0))
	assert.Empty(t, store.Query("", 0))
}

// TestPropTracker_ChassisDelete asserts a Chassis delete event drops the
// tracker's per-chassis bookkeeping.
func TestPropTracker_ChassisDelete(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 5, 10200)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(events.NewHub(), store, nbClient, sbClient)
	tracker.seed(context.Background())

	require.Contains(t, tracker.hostnames, "ch-1")
	tracker.handleEvent(context.Background(), events.Event{
		Type:     events.EventDelete,
		Database: "sb",
		Table:    "Chassis",
		Row:      map[string]any{"name": "ch-1"},
	})
	assert.NotContains(t, tracker.hostnames, "ch-1")
	assert.NotContains(t, tracker.chassisLastGen, "ch-1")
}

// TestPropTracker_StopIdempotent asserts the returned stop function is safe to
// call more than once (it must not panic on a double close(done)).
func TestPropTracker_StopIdempotent(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(events.NewHub(), store, nbClient, sbClient)
	stop := tracker.Start(context.Background())

	assert.NotPanics(t, func() {
		stop()
		stop()
	})
}

// TestPropTracker_NoStartupGap asserts an event published immediately after
// Start returns is delivered and tracked — the consumer goroutine is running
// and the subscription is live, so there is no gap where startup events are
// dropped.
func TestPropTracker_NoStartupGap(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 4, 9500)

	hub := events.NewHub()
	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(hub, store, nbClient, sbClient)
	stop := tracker.Start(context.Background())
	defer stop()

	hub.Publish(chassisPrivateEvent("ch-1", 5, 10300))

	require.Eventually(t, func() bool {
		return len(store.Query("ch-1", 0)) == 1
	}, 5*time.Second, 20*time.Millisecond)
}
