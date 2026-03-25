package telemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPropagationStore_AddAndQuery(t *testing.T) {
	store := NewPropagationStore(100, time.Hour)

	now := time.Now().UnixMilli()
	store.Add(PropagationEvent{
		Generation: 1, Chassis: "ch-1", Hostname: "host-1",
		NbTimestampMs: now - 500, ChassisTimestampMs: now - 200,
		LatencyMs: 300, RecordedAt: now,
	})
	store.Add(PropagationEvent{
		Generation: 1, Chassis: "ch-2", Hostname: "host-2",
		NbTimestampMs: now - 500, ChassisTimestampMs: now - 100,
		LatencyMs: 400, RecordedAt: now + 1,
	})
	store.Add(PropagationEvent{
		Generation: 2, Chassis: "ch-1", Hostname: "host-1",
		NbTimestampMs: now, ChassisTimestampMs: now + 150,
		LatencyMs: 150, RecordedAt: now + 2,
	})

	// Query all
	all := store.Query("", 0)
	assert.Len(t, all, 3)

	// Filter by chassis
	ch1 := store.Query("ch-1", 0)
	assert.Len(t, ch1, 2)
	for _, e := range ch1 {
		assert.Equal(t, "ch-1", e.Chassis)
	}

	// Filter by since
	recent := store.Query("", now+1)
	assert.Len(t, recent, 2)

	// Filter by both
	ch2Recent := store.Query("ch-2", now+1)
	assert.Len(t, ch2Recent, 1)
	assert.Equal(t, "ch-2", ch2Recent[0].Chassis)
}

func TestPropagationStore_PruneBySize(t *testing.T) {
	store := NewPropagationStore(3, 0)

	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		store.Add(PropagationEvent{
			Generation: i, Chassis: "ch-1",
			LatencyMs: int64(i * 100), RecordedAt: now + int64(i),
		})
	}

	all := store.Query("", 0)
	require.Len(t, all, 3)
	// Should keep the last 3
	assert.Equal(t, 2, all[0].Generation)
	assert.Equal(t, 3, all[1].Generation)
	assert.Equal(t, 4, all[2].Generation)
}

func TestPropagationStore_PruneByAge(t *testing.T) {
	store := NewPropagationStore(100, 500*time.Millisecond)

	now := time.Now().UnixMilli()
	// Add an old event
	store.Add(PropagationEvent{
		Generation: 1, Chassis: "ch-1",
		LatencyMs: 100, RecordedAt: now - 1000,
	})
	// Add a recent event — this triggers pruning
	store.Add(PropagationEvent{
		Generation: 2, Chassis: "ch-1",
		LatencyMs: 200, RecordedAt: now,
	})

	all := store.Query("", 0)
	require.Len(t, all, 1)
	assert.Equal(t, 2, all[0].Generation)
}

func TestPropagationStore_Summary(t *testing.T) {
	store := NewPropagationStore(100, time.Hour)

	now := time.Now().UnixMilli()
	// ch-1: latencies 100, 200, 300, 400, 500
	for i := 1; i <= 5; i++ {
		store.Add(PropagationEvent{
			Generation: i, Chassis: "ch-1", Hostname: "host-1",
			LatencyMs: int64(i * 100), RecordedAt: now + int64(i),
		})
	}
	// ch-2: latencies 1000, 2000, 3000
	for i := 1; i <= 3; i++ {
		store.Add(PropagationEvent{
			Generation: i, Chassis: "ch-2", Hostname: "host-2",
			LatencyMs: int64(i * 1000), RecordedAt: now + int64(i),
		})
	}

	summaries := store.Summary(0)
	require.Len(t, summaries, 2)

	// Sorted by P95 descending — ch-2 should be first
	assert.Equal(t, "ch-2", summaries[0].Chassis)
	assert.Equal(t, "ch-1", summaries[1].Chassis)

	// ch-2 stats
	ch2 := summaries[0]
	assert.Equal(t, 3, ch2.Count)
	assert.Equal(t, int64(1000), ch2.MinMs)
	assert.Equal(t, int64(3000), ch2.MaxMs)
	assert.InDelta(t, 2000.0, ch2.AvgMs, 0.01)
	assert.Equal(t, float64(2000), ch2.P50Ms)

	// ch-1 stats
	ch1 := summaries[1]
	assert.Equal(t, 5, ch1.Count)
	assert.Equal(t, int64(100), ch1.MinMs)
	assert.Equal(t, int64(500), ch1.MaxMs)
	assert.InDelta(t, 300.0, ch1.AvgMs, 0.01)
	assert.Equal(t, float64(300), ch1.P50Ms)
}

func TestPropagationStore_SummaryWithSince(t *testing.T) {
	store := NewPropagationStore(100, time.Hour)

	now := time.Now().UnixMilli()
	store.Add(PropagationEvent{
		Generation: 1, Chassis: "ch-1", Hostname: "host-1",
		LatencyMs: 100, RecordedAt: now - 5000,
	})
	store.Add(PropagationEvent{
		Generation: 2, Chassis: "ch-1", Hostname: "host-1",
		LatencyMs: 200, RecordedAt: now,
	})

	// Summary since now-1000: should only include the second event
	summaries := store.Summary(now - 1000)
	require.Len(t, summaries, 1)
	assert.Equal(t, 1, summaries[0].Count)
	assert.InDelta(t, 200.0, summaries[0].AvgMs, 0.01)
}

func TestPropagationStore_EmptyStore(t *testing.T) {
	store := NewPropagationStore(100, time.Hour)

	assert.Empty(t, store.Query("", 0))
	assert.Empty(t, store.Query("ch-1", 0))
	assert.Empty(t, store.Summary(0))
}

func TestPercentile(t *testing.T) {
	tests := []struct {
		name   string
		sorted []int64
		p      int
		want   int64
	}{
		{"empty", nil, 50, 0},
		{"single", []int64{100}, 50, 100},
		{"single p99", []int64{100}, 99, 100},
		{"two values p50", []int64{100, 200}, 50, 100},
		{"two values p99", []int64{100, 200}, 99, 200},
		{"five values p50", []int64{100, 200, 300, 400, 500}, 50, 300},
		{"five values p95", []int64{100, 200, 300, 400, 500}, 95, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, percentile(tt.sorted, tt.p))
		})
	}
}
