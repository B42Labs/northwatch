package flowdiff

import (
	"sync"
	"time"
)

// FlowChange represents a single LogicalFlow change event.
type FlowChange struct {
	Timestamp int64          `json:"timestamp"` // Unix millis
	Type      string         `json:"type"`      // "insert", "update", "delete"
	UUID      string         `json:"uuid"`
	OldRow    map[string]any `json:"old_row,omitempty"`
	NewRow    map[string]any `json:"new_row,omitempty"`
	Datapath  string         `json:"datapath,omitempty"`
}

// Store is a fixed-capacity circular buffer for flow changes with time-based
// pruning. Logical_Flow is OVN's highest-churn table, so the backing array is
// preallocated once and Add overwrites the oldest entry in place at capacity —
// avoiding the two O(maxSize) copies the previous slice implementation paid on
// every insert.
type Store struct {
	mu      sync.RWMutex
	buf     []FlowChange // fixed capacity == maxSize
	start   int          // index of the oldest change
	count   int          // number of valid changes, 0 <= count <= maxSize
	maxSize int
	maxAge  time.Duration
}

// NewStore creates a Store with the given capacity and max age. maxSize is
// clamped to a minimum of 1 so the ring buffer is always valid.
func NewStore(maxSize int, maxAge time.Duration) *Store {
	if maxSize < 1 {
		maxSize = 1
	}
	return &Store{
		buf:     make([]FlowChange, maxSize),
		maxSize: maxSize,
		maxAge:  maxAge,
	}
}

// Add stores a change, overwriting the oldest entry when at capacity, and prunes
// entries past maxAge from the front.
func (s *Store) Add(change FlowChange) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf[(s.start+s.count)%s.maxSize] = change
	if s.count < s.maxSize {
		s.count++
	} else {
		s.start = (s.start + 1) % s.maxSize
	}
	s.pruneAgeLocked()
}

// Query returns changes matching the optional datapath filter and since
// timestamp, in oldest-to-newest order. Entries older than maxAge are filtered
// out here too, so a stale change never surfaces just because no Add has pruned
// it yet.
func (s *Store) Query(datapath string, since int64) []FlowChange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ageCutoff int64
	if s.maxAge > 0 {
		ageCutoff = time.Now().UnixMilli() - s.maxAge.Milliseconds()
	}

	var result []FlowChange
	for i := 0; i < s.count; i++ {
		c := s.buf[(s.start+i)%s.maxSize]
		if c.Timestamp < ageCutoff {
			continue
		}
		if since > 0 && c.Timestamp < since {
			continue
		}
		if datapath != "" && c.Datapath != datapath {
			continue
		}
		result = append(result, c)
	}
	return result
}

// pruneAgeLocked drops changes older than maxAge from the front of the ring.
// Size pruning is inherent to the fixed-capacity buffer.
func (s *Store) pruneAgeLocked() {
	if s.maxAge <= 0 {
		return
	}
	cutoff := time.Now().UnixMilli() - s.maxAge.Milliseconds()
	for s.count > 0 && s.buf[s.start].Timestamp < cutoff {
		s.start = (s.start + 1) % s.maxSize
		s.count--
	}
}
