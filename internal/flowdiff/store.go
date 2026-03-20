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

// Store is a bounded ring buffer for flow changes with time-based pruning.
type Store struct {
	mu      sync.RWMutex
	changes []FlowChange
	maxSize int
	maxAge  time.Duration
}

// NewStore creates a Store with the given capacity and max age.
func NewStore(maxSize int, maxAge time.Duration) *Store {
	return &Store{
		changes: make([]FlowChange, 0, maxSize),
		maxSize: maxSize,
		maxAge:  maxAge,
	}
}

// Add appends a change and prunes old entries.
func (s *Store) Add(change FlowChange) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.changes = append(s.changes, change)
	s.pruneLocked()
}

// Query returns changes matching the optional datapath filter and since timestamp.
func (s *Store) Query(datapath string, since int64) []FlowChange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []FlowChange
	for _, c := range s.changes {
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

func (s *Store) pruneLocked() {
	start := 0

	// Prune by age
	if s.maxAge > 0 {
		cutoff := time.Now().UnixMilli() - s.maxAge.Milliseconds()
		for start < len(s.changes) && s.changes[start].Timestamp < cutoff {
			start++
		}
	}

	// Prune by size
	remaining := len(s.changes) - start
	if s.maxSize > 0 && remaining > s.maxSize {
		start = len(s.changes) - s.maxSize
	}

	// Copy to a new slice to release the old backing array
	if start > 0 {
		pruned := make([]FlowChange, len(s.changes)-start)
		copy(pruned, s.changes[start:])
		s.changes = pruned
	}
}
