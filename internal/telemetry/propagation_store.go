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

// PropagationStore is a fixed-capacity circular buffer for propagation events
// with time-based pruning. The backing array is preallocated once; adding an
// event at capacity overwrites the oldest entry in place, so a steady stream of
// events costs O(1) per Add instead of reallocating and copying the whole
// buffer on every insert.
type PropagationStore struct {
	mu      sync.RWMutex
	buf     []PropagationEvent // fixed capacity == maxSize
	start   int                // index of the oldest event
	count   int                // number of valid events, 0 <= count <= maxSize
	maxSize int
	maxAge  time.Duration
}

// NewPropagationStore creates a PropagationStore with the given capacity and max
// age. maxSize is clamped to a minimum of 1 so the ring buffer is always valid.
func NewPropagationStore(maxSize int, maxAge time.Duration) *PropagationStore {
	if maxSize < 1 {
		maxSize = 1
	}
	return &PropagationStore{
		buf:     make([]PropagationEvent, maxSize),
		maxSize: maxSize,
		maxAge:  maxAge,
	}
}

// Add stores an event, overwriting the oldest entry when at capacity, and prunes
// entries past maxAge from the front.
func (s *PropagationStore) Add(event PropagationEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf[(s.start+s.count)%s.maxSize] = event
	if s.count < s.maxSize {
		s.count++
	} else {
		// Full: the write above overwrote the oldest entry; advance start.
		s.start = (s.start + 1) % s.maxSize
	}
	s.pruneAgeLocked()
}

// Query returns events matching the optional chassis filter and since timestamp,
// in oldest-to-newest order.
func (s *PropagationStore) Query(chassis string, since int64) []PropagationEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []PropagationEvent
	for i := 0; i < s.count; i++ {
		e := s.buf[(s.start+i)%s.maxSize]
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

	for i := 0; i < s.count; i++ {
		e := s.buf[(s.start+i)%s.maxSize]
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

// pruneAgeLocked drops events older than maxAge from the front of the ring. Size
// pruning is inherent to the fixed-capacity buffer (Add overwrites the oldest
// entry), so only age pruning needs an explicit pass.
func (s *PropagationStore) pruneAgeLocked() {
	if s.maxAge <= 0 {
		return
	}
	cutoff := time.Now().UnixMilli() - s.maxAge.Milliseconds()
	for s.count > 0 && s.buf[s.start].RecordedAt < cutoff {
		s.start = (s.start + 1) % s.maxSize
		s.count--
	}
}
