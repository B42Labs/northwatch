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

func TestPropTracker_CatchUp(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	hub := events.NewHub()
	nbClient.Cache().AddEventHandler(events.NewBridge(hub, "nb"))
	sbClient.Cache().AddEventHandler(events.NewBridge(hub, "sb"))

	// Seed: NB_Global with nb_cfg=5, timestamp=10000
	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)

	// Seed: Chassis and Chassis_Private at nb_cfg=4 (behind)
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 4, 9500)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(hub, store, nbClient, sbClient)
	stop := tracker.Start(context.Background())
	defer stop()

	// Wait for seeding
	time.Sleep(100 * time.Millisecond)

	// Simulate chassis catching up: update Chassis_Private nb_cfg to 5
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 5, 10300)

	// Wait for event processing
	require.Eventually(t, func() bool {
		return len(store.Query("", 0)) > 0
	}, 5*time.Second, 50*time.Millisecond)

	evts := store.Query("ch-1", 0)
	require.Len(t, evts, 1)
	assert.Equal(t, 5, evts[0].Generation)
	assert.Equal(t, "ch-1", evts[0].Chassis)
	assert.Equal(t, "host-1", evts[0].Hostname)
	assert.Equal(t, int64(10000), evts[0].NbTimestampMs)
	assert.Equal(t, int64(10300), evts[0].ChassisTimestampMs)
	assert.Equal(t, int64(300), evts[0].LatencyMs)
}

func TestPropTracker_GenBump(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	hub := events.NewHub()
	nbClient.Cache().AddEventHandler(events.NewBridge(hub, "nb"))
	sbClient.Cache().AddEventHandler(events.NewBridge(hub, "sb"))

	// Seed: NB_Global at gen 5
	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)

	// Seed: Chassis and Chassis_Private already at gen 5
	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 5, 10200)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(hub, store, nbClient, sbClient)
	stop := tracker.Start(context.Background())
	defer stop()

	time.Sleep(100 * time.Millisecond)

	// Bump NB_Global to gen 6 — chassis is still at gen 5
	testutil.UpdateNBGlobal(t, nbClient, 6, 20000)
	time.Sleep(100 * time.Millisecond)

	// Now chassis catches up to gen 6
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 6, 20500)

	require.Eventually(t, func() bool {
		return len(store.Query("", 0)) > 0
	}, 5*time.Second, 50*time.Millisecond)

	evts := store.Query("ch-1", 0)
	require.Len(t, evts, 1)
	assert.Equal(t, 6, evts[0].Generation)
	assert.Equal(t, int64(500), evts[0].LatencyMs)
}

func TestPropTracker_NoDup(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	hub := events.NewHub()
	nbClient.Cache().AddEventHandler(events.NewBridge(hub, "nb"))
	sbClient.Cache().AddEventHandler(events.NewBridge(hub, "sb"))

	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 4, 9500)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(hub, store, nbClient, sbClient)
	stop := tracker.Start(context.Background())
	defer stop()

	time.Sleep(100 * time.Millisecond)

	// First catch-up
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 5, 10300)
	require.Eventually(t, func() bool {
		return len(store.Query("ch-1", 0)) == 1
	}, 5*time.Second, 50*time.Millisecond)

	// Second update at same generation
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 5, 10300)
	time.Sleep(200 * time.Millisecond)

	// Should still only have 1 event
	assert.Len(t, store.Query("ch-1", 0), 1)
}

func TestPropTracker_StaleTimestamp(t *testing.T) {
	// Simulate the real OVN flow where CMS increments nb_cfg and
	// ovn-northd updates nb_cfg_timestamp in separate transactions.
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	hub := events.NewHub()
	nbClient.Cache().AddEventHandler(events.NewBridge(hub, "nb"))
	sbClient.Cache().AddEventHandler(events.NewBridge(hub, "sb"))

	// Seed: gen 5, timestamp 10000
	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)
	testutil.UpdateNBGlobal(t, nbClient, 5, 10000)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 5, 10200)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(hub, store, nbClient, sbClient)
	stop := tracker.Start(context.Background())
	defer stop()

	time.Sleep(100 * time.Millisecond)

	// Step 1: CMS bumps gen to 6, but timestamp is still from gen 5 (stale)
	testutil.UpdateNBGlobal(t, nbClient, 6, 10000)
	time.Sleep(100 * time.Millisecond)

	// Step 2: Chassis catches up to gen 6
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 6, 20500)
	time.Sleep(200 * time.Millisecond)

	// No event should be recorded yet — NB timestamp is stale
	assert.Empty(t, store.Query("", 0), "should not record with stale NB timestamp")

	// Step 3: ovn-northd finishes and writes the real timestamp
	testutil.UpdateNBGlobal(t, nbClient, 6, 20000)

	require.Eventually(t, func() bool {
		return len(store.Query("", 0)) > 0
	}, 5*time.Second, 50*time.Millisecond)

	evts := store.Query("ch-1", 0)
	require.Len(t, evts, 1)
	assert.Equal(t, 6, evts[0].Generation)
	assert.Equal(t, int64(20000), evts[0].NbTimestampMs)
	assert.Equal(t, int64(500), evts[0].LatencyMs)
}

func TestPropTracker_ZeroTs(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	hub := events.NewHub()
	nbClient.Cache().AddEventHandler(events.NewBridge(hub, "nb"))
	sbClient.Cache().AddEventHandler(events.NewBridge(hub, "sb"))

	// NB_Global with zero timestamp
	testutil.InsertNBGlobal(t, nbClient, 5, 0, 0)

	chassisUUID := testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")
	testutil.InsertChassisPrivate(t, sbClient, "ch-1", &chassisUUID, 4, 0)

	store := NewPropagationStore(100, time.Hour)
	tracker := NewPropagationTracker(hub, store, nbClient, sbClient)
	stop := tracker.Start(context.Background())
	defer stop()

	time.Sleep(100 * time.Millisecond)

	// Chassis catches up but with zero timestamps — should be skipped
	testutil.UpdateChassisPrivate(t, sbClient, "ch-1", 5, 0)
	time.Sleep(200 * time.Millisecond)

	assert.Empty(t, store.Query("", 0))
}
