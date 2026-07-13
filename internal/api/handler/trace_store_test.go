package handler

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTraceStore(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	require.NotNil(t, store)
	assert.Empty(t, store.List())
}

func TestTraceStore_StoreAndGet(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	trace := TraceResponse{
		PortUUID:     "port-1",
		PortName:     "lsp-vm1",
		DatapathUUID: "dp-1",
		DatapathName: "sw0",
	}

	store.Store("trace-1", trace)

	got, ok := store.Get("trace-1")
	require.True(t, ok)
	assert.Equal(t, "trace-1", got.ID)
	assert.Equal(t, "lsp-vm1", got.Trace.PortName)
	assert.Equal(t, "dp-1", got.Trace.DatapathUUID)
}

func TestTraceStore_GetNotFound(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)

	_, ok := store.Get("nonexistent")
	assert.False(t, ok)
}

func TestTraceStore_GetExpired(t *testing.T) {
	store := NewTraceStore(1 * time.Minute)
	base := time.Now()
	current := base
	store.now = func() time.Time { return current }

	store.Store("trace-1", TraceResponse{PortName: "p1"})

	// Advance the clock past maxAge instead of sleeping.
	current = base.Add(2 * time.Minute)

	_, ok := store.Get("trace-1")
	assert.False(t, ok)
}

func TestTraceStore_List(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	store.Store("t1", TraceResponse{PortName: "p1"})
	store.Store("t2", TraceResponse{PortName: "p2"})

	traces := store.List()
	assert.Len(t, traces, 2)
}

func TestTraceStore_ListFiltersExpired(t *testing.T) {
	store := NewTraceStore(1 * time.Minute)
	base := time.Now()
	current := base
	store.now = func() time.Time { return current }

	store.Store("t1", TraceResponse{PortName: "p1"})

	// t1 ages past maxAge; t2 is stored fresh at the advanced clock.
	current = base.Add(2 * time.Minute)
	store.Store("t2", TraceResponse{PortName: "p2"})

	traces := store.List()
	assert.Len(t, traces, 1)
	assert.Equal(t, "t2", traces[0].ID)
}

func TestTraceStore_StoreCleanupExpired(t *testing.T) {
	store := NewTraceStore(1 * time.Minute)
	base := time.Now()
	current := base
	store.now = func() time.Time { return current }

	store.Store("old", TraceResponse{PortName: "old"})

	// Advance past maxAge so "old" is eligible for the amortized sweep.
	current = base.Add(2 * time.Minute)

	// Cleanup is amortized over cleanupEvery stores. Drive enough writes
	// to guarantee one sweep occurs and removes the expired entry.
	for i := 0; i < cleanupEvery; i++ {
		store.Store("filler", TraceResponse{PortName: "filler"})
	}

	store.mu.RLock()
	_, hasOld := store.traces["old"]
	store.mu.RUnlock()
	assert.False(t, hasOld, "expired trace should be cleaned up after amortized sweep")
}

func TestTraceStore_Overwrite(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	store.Store("t1", TraceResponse{PortName: "original"})
	store.Store("t1", TraceResponse{PortName: "updated"})

	got, ok := store.Get("t1")
	require.True(t, ok)
	assert.Equal(t, "updated", got.Trace.PortName)
}

// TestTraceStore_EvictsOldestAtCapacity pins the entry cap. Age alone was not a
// bound: a full flow dump was kept for an hour with no limit on how many, so a
// client could pin unbounded memory by requesting traces in a loop.
func TestTraceStore_EvictsOldestAtCapacity(t *testing.T) {
	now := time.Now()
	store := NewTraceStore(1 * time.Hour)
	store.now = func() time.Time { return now }

	for i := range maxTraceEntries {
		now = now.Add(time.Second)
		store.Store(fmt.Sprintf("trace-%03d", i), TraceResponse{PortName: fmt.Sprintf("port-%d", i)})
	}
	require.Len(t, store.List(), maxTraceEntries)

	// One more store evicts the oldest, keeping the store at its cap.
	now = now.Add(time.Second)
	store.Store("trace-new", TraceResponse{PortName: "port-new"})

	assert.Len(t, store.List(), maxTraceEntries)

	_, ok := store.Get("trace-000")
	assert.False(t, ok, "the oldest trace must be evicted")

	_, ok = store.Get("trace-new")
	assert.True(t, ok)
	_, ok = store.Get("trace-001")
	assert.True(t, ok, "the second-oldest must survive")
}

// TestTraceStore_ReplaceDoesNotEvict guards the eviction path against dropping a
// trace when an existing id is merely overwritten.
func TestTraceStore_ReplaceDoesNotEvict(t *testing.T) {
	now := time.Now()
	store := NewTraceStore(1 * time.Hour)
	store.now = func() time.Time { return now }

	for i := range maxTraceEntries {
		now = now.Add(time.Second)
		store.Store(fmt.Sprintf("trace-%03d", i), TraceResponse{PortName: fmt.Sprintf("port-%d", i)})
	}

	now = now.Add(time.Second)
	store.Store("trace-000", TraceResponse{PortName: "port-replaced"})

	assert.Len(t, store.List(), maxTraceEntries)
	stored, ok := store.Get("trace-000")
	require.True(t, ok)
	assert.Equal(t, "port-replaced", stored.Trace.PortName)
}
