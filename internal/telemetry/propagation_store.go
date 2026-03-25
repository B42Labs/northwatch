package telemetry

import (
	"math"
	"sort"
	"sync"
	"time"
)

// PropagationEvent records when a single chassis caught up to a given nb_cfg generation.
type PropagationEvent struct {
	Generation         int    `json:"generation"`
	NbTimestampMs      int64  `json:"nb_timestamp_ms"`
	Chassis            string `json:"chassis"`
	Hostname           string `json:"hostname"`
	ChassisTimestampMs int64  `json:"chassis_timestamp_ms"`
	LatencyMs          int64  `json:"latency_ms"`
	RecordedAt         int64  `json:"recorded_at"`
}

// ChassisSummary holds aggregated propagation statistics for a single chassis.
type ChassisSummary struct {
	Chassis  string  `json:"chassis"`
	Hostname string  `json:"hostname"`
	Count    int     `json:"count"`
	AvgMs    float64 `json:"avg_ms"`
	P50Ms    float64 `json:"p50_ms"`
	P95Ms    float64 `json:"p95_ms"`
	P99Ms    float64 `json:"p99_ms"`
	MaxMs    int64   `json:"max_ms"`
	MinMs    int64   `json:"min_ms"`
}

// PropagationStore is a bounded ring buffer for propagation events with time-based pruning.
type PropagationStore struct {
	mu      sync.RWMutex
	events  []PropagationEvent
	maxSize int
	maxAge  time.Duration
}

// NewPropagationStore creates a PropagationStore with the given capacity and max age.
func NewPropagationStore(maxSize int, maxAge time.Duration) *PropagationStore {
	return &PropagationStore{
		events:  make([]PropagationEvent, 0, maxSize),
		maxSize: maxSize,
		maxAge:  maxAge,
	}
}

// Add appends an event and prunes old entries.
func (s *PropagationStore) Add(event PropagationEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)
	s.pruneLocked()
}

// Query returns events matching the optional chassis filter and since timestamp.
func (s *PropagationStore) Query(chassis string, since int64) []PropagationEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []PropagationEvent
	for _, e := range s.events {
		if since > 0 && e.RecordedAt < since {
			continue
		}
		if chassis != "" && e.Chassis != chassis {
			continue
		}
		result = append(result, e)
	}
	return result
}

// Summary returns aggregated propagation statistics per chassis for events since the given timestamp.
func (s *PropagationStore) Summary(since int64) []ChassisSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Group latencies by chassis
	type chassisData struct {
		hostname  string
		latencies []int64
	}
	groups := make(map[string]*chassisData)

	for _, e := range s.events {
		if since > 0 && e.RecordedAt < since {
			continue
		}
		d, ok := groups[e.Chassis]
		if !ok {
			d = &chassisData{hostname: e.Hostname}
			groups[e.Chassis] = d
		}
		d.latencies = append(d.latencies, e.LatencyMs)
		// Keep latest hostname
		if e.Hostname != "" {
			d.hostname = e.Hostname
		}
	}

	result := make([]ChassisSummary, 0, len(groups))
	for name, d := range groups {
		if len(d.latencies) == 0 {
			continue
		}
		sort.Slice(d.latencies, func(i, j int) bool { return d.latencies[i] < d.latencies[j] })

		var sum int64
		for _, v := range d.latencies {
			sum += v
		}

		result = append(result, ChassisSummary{
			Chassis:  name,
			Hostname: d.hostname,
			Count:    len(d.latencies),
			AvgMs:    math.Round(float64(sum)/float64(len(d.latencies))*100) / 100,
			P50Ms:    float64(percentile(d.latencies, 50)),
			P95Ms:    float64(percentile(d.latencies, 95)),
			P99Ms:    float64(percentile(d.latencies, 99)),
			MaxMs:    d.latencies[len(d.latencies)-1],
			MinMs:    d.latencies[0],
		})
	}

	// Sort by P95 descending (worst performers first)
	sort.Slice(result, func(i, j int) bool { return result[i].P95Ms > result[j].P95Ms })

	return result
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (s *PropagationStore) pruneLocked() {
	start := 0

	// Prune by age
	if s.maxAge > 0 {
		cutoff := time.Now().UnixMilli() - s.maxAge.Milliseconds()
		for start < len(s.events) && s.events[start].RecordedAt < cutoff {
			start++
		}
	}

	// Prune by size
	remaining := len(s.events) - start
	if s.maxSize > 0 && remaining > s.maxSize {
		start = len(s.events) - s.maxSize
	}

	// Copy to a new slice to release the old backing array
	if start > 0 {
		pruned := make([]PropagationEvent, len(s.events)-start)
		copy(pruned, s.events[start:])
		s.events = pruned
	}
}
