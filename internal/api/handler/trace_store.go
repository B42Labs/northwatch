package handler

import (
	"sync"
	"time"
)

// TraceStore stores packet traces for later retrieval and export.
type TraceStore struct {
	mu     sync.RWMutex
	traces map[string]StoredTrace
	maxAge time.Duration
}

// StoredTrace wraps a TraceResponse with metadata.
type StoredTrace struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	Trace     TraceResponse `json:"trace"`
}

// NewTraceStore creates a new TraceStore with the given max age.
func NewTraceStore(maxAge time.Duration) *TraceStore {
	return &TraceStore{
		traces: make(map[string]StoredTrace),
		maxAge: maxAge,
	}
}

// Store persists a trace and returns its ID.
func (s *TraceStore) Store(id string, trace TraceResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for k, v := range s.traces {
		if now.Sub(v.CreatedAt) > s.maxAge {
			delete(s.traces, k)
		}
	}

	s.traces[id] = StoredTrace{
		ID:        id,
		CreatedAt: now,
		Trace:     trace,
	}
}

// Get retrieves a stored trace by ID.
func (s *TraceStore) Get(id string) (StoredTrace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.traces[id]
	if ok && time.Since(t.CreatedAt) > s.maxAge {
		return StoredTrace{}, false
	}
	return t, ok
}

// List returns all stored traces.
func (s *TraceStore) List() []StoredTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var result []StoredTrace
	for _, t := range s.traces {
		if now.Sub(t.CreatedAt) <= s.maxAge {
			result = append(result, t)
		}
	}
	return result
}
