package handler

import (
	"sync"
	"time"
)

// cleanupEvery determines how often Store sweeps expired traces.
// One sweep per N stores keeps the work amortized so individual writes
// remain O(1) on average instead of O(n) per call.
const cleanupEvery = 64

// maxTraceEntries bounds how many traces are held at once. Age alone was not a
// bound: a trace is a full flow dump, and traces were kept for an hour with no
// limit on how many, so an unauthenticated client could pin an unbounded amount
// of memory just by requesting traces in a loop.
const maxTraceEntries = 200

// TraceStore stores packet traces for later retrieval and export.
type TraceStore struct {
	mu          sync.RWMutex
	traces      map[string]StoredTrace
	maxAge      time.Duration
	storesSince int
	// now returns the current time; a field so tests can drive expiry
	// deterministically. It defaults to time.Now and is read under mu.
	now func() time.Time
}

// StoredTrace wraps a TraceResponse with metadata.
type StoredTrace struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	Trace     TraceResponse `json:"trace"`
}

// NewTraceStore creates a new TraceStore holding at most maxTraceEntries traces,
// each for the given max age.
func NewTraceStore(maxAge time.Duration) *TraceStore {
	return &TraceStore{
		traces: make(map[string]StoredTrace),
		maxAge: maxAge,
		now:    time.Now,
	}
}

// Store persists a trace under the given ID, evicting the oldest trace when the
// store is full.
func (s *TraceStore) Store(id string, trace TraceResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.storesSince++
	if s.storesSince >= cleanupEvery {
		s.storesSince = 0
		for k, v := range s.traces {
			if now.Sub(v.CreatedAt) > s.maxAge {
				delete(s.traces, k)
			}
		}
	}

	if _, replacing := s.traces[id]; !replacing {
		s.evictOldest(now)
	}

	s.traces[id] = StoredTrace{
		ID:        id,
		CreatedAt: now,
		Trace:     trace,
	}
}

// evictOldest makes room for one new trace: it drops expired entries first and,
// if the store is still at capacity, the oldest remaining one. Callers hold mu.
func (s *TraceStore) evictOldest(now time.Time) {
	if len(s.traces) < maxTraceEntries {
		return
	}

	for k, v := range s.traces {
		if now.Sub(v.CreatedAt) > s.maxAge {
			delete(s.traces, k)
		}
	}
	if len(s.traces) < maxTraceEntries {
		return
	}

	var oldestID string
	var oldestAt time.Time
	for k, v := range s.traces {
		if oldestID == "" || v.CreatedAt.Before(oldestAt) {
			oldestID, oldestAt = k, v.CreatedAt
		}
	}
	delete(s.traces, oldestID)
}

// Get retrieves a stored trace by ID.
func (s *TraceStore) Get(id string) (StoredTrace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.traces[id]
	if ok && s.now().Sub(t.CreatedAt) > s.maxAge {
		return StoredTrace{}, false
	}
	return t, ok
}

// List returns all stored traces.
func (s *TraceStore) List() []StoredTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	var result []StoredTrace
	for _, t := range s.traces {
		if now.Sub(t.CreatedAt) <= s.maxAge {
			result = append(result, t)
		}
	}
	return result
}
