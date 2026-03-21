package handler

import (
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
	store := NewTraceStore(1 * time.Millisecond)
	store.Store("trace-1", TraceResponse{PortName: "p1"})

	time.Sleep(5 * time.Millisecond)

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
	store := NewTraceStore(1 * time.Millisecond)
	store.Store("t1", TraceResponse{PortName: "p1"})

	time.Sleep(5 * time.Millisecond)

	store.Store("t2", TraceResponse{PortName: "p2"})

	traces := store.List()
	assert.Len(t, traces, 1)
	assert.Equal(t, "t2", traces[0].ID)
}

func TestTraceStore_StoreCleanupExpired(t *testing.T) {
	store := NewTraceStore(1 * time.Millisecond)
	store.Store("old", TraceResponse{PortName: "old"})

	time.Sleep(5 * time.Millisecond)

	// Storing a new trace triggers cleanup of expired entries
	store.Store("new", TraceResponse{PortName: "new"})

	store.mu.RLock()
	_, hasOld := store.traces["old"]
	store.mu.RUnlock()
	assert.False(t, hasOld, "expired trace should be cleaned up on Store")
}

func TestTraceStore_Overwrite(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	store.Store("t1", TraceResponse{PortName: "original"})
	store.Store("t1", TraceResponse{PortName: "updated"})

	got, ok := store.Get("t1")
	require.True(t, ok)
	assert.Equal(t, "updated", got.Trace.PortName)
}
