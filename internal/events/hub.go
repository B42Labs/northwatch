package events

import (
	"log/slog"
	"sync"
	"time"
)

const subscriberBufferSize = 256

// dropLogInterval bounds how often a slow subscriber's dropped-event summary is
// logged, so one stuck WebSocket client cannot flood the log with a line per
// dropped event (thousands per second on a busy cluster).
const dropLogInterval = 10 * time.Second

// Subscriber represents a connected WebSocket client that receives events.
type Subscriber struct {
	C       chan Event
	id      uint64
	mu      sync.RWMutex
	filters []Filter
	// dropped counts events discarded because the buffer was full since the last
	// summary; lastDropLog is when that summary was last emitted. Both are
	// guarded by mu.
	dropped     uint64
	lastDropLog time.Time
}

// recordDrop accounts for one dropped event and reports whether a summary should
// be logged now (at most once per dropLogInterval). When it returns true the
// accumulated count is returned and reset to zero.
func (s *Subscriber) recordDrop(now time.Time) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropped++
	if s.lastDropLog.IsZero() || now.Sub(s.lastDropLog) >= dropLogInterval {
		n := s.dropped
		s.dropped = 0
		s.lastDropLog = now
		return n, true
	}
	return 0, false
}

// AddFilter adds a subscription filter.
func (s *Subscriber) AddFilter(f Filter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filters = append(s.filters, f)
}

// RemoveFilter removes filters matching the given database and tables.
func (s *Subscriber) RemoveFilter(database string, tables []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tableSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		tableSet[t] = true
	}

	filtered := s.filters[:0]
	for _, f := range s.filters {
		if f.Database == database && matchesTables(f.Tables, tableSet) {
			continue
		}
		filtered = append(filtered, f)
	}
	s.filters = filtered
}

func matchesTables(a []string, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for _, t := range a {
		if !b[t] {
			return false
		}
	}
	return true
}

// matches returns true if any filter matches the event.
func (s *Subscriber) matches(e Event) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.filters {
		if f.Matches(e) {
			return true
		}
	}
	return false
}

// Hub is an in-process pub/sub hub for OVSDB events.
//
// Lock ordering: Hub.mu must be acquired before Subscriber.mu.
// Publish holds Hub.mu (RLock) then calls Subscriber.matches which takes Subscriber.mu.
// No code path may acquire Hub.mu while holding Subscriber.mu.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[uint64]*Subscriber
	nextID      uint64
}

// NewHub creates a new event hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[uint64]*Subscriber),
	}
}

// Subscribe creates a new subscriber and returns it.
func (h *Hub) Subscribe() *Subscriber {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	s := &Subscriber{
		C:  make(chan Event, subscriberBufferSize),
		id: h.nextID,
	}
	h.subscribers[s.id] = s
	return s
}

// Unsubscribe removes a subscriber and closes its channel.
func (h *Hub) Unsubscribe(s *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscribers[s.id]; ok {
		delete(h.subscribers, s.id)
		close(s.C)
	}
}

// Publish sends an event to all matching subscribers.
// If a subscriber's buffer is full, the event is dropped for that subscriber.
func (h *Hub) Publish(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		if !s.matches(e) {
			continue
		}
		select {
		case s.C <- e:
		default:
			if n, ok := s.recordDrop(time.Now()); ok {
				slog.Warn("events: dropping events for slow subscriber",
					"subscriber", s.id, "dropped_since_last_report", n)
			}
		}
	}
}

// HasSubscriberFor reports whether any subscriber's filters would accept an
// event for the given database and table. The bridge uses it to skip the
// per-row reflection conversion when nobody is listening for a table.
func (h *Hub) HasSubscriberFor(database, table string) bool {
	probe := Event{Database: database, Table: table}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		if s.matches(probe) {
			return true
		}
	}
	return false
}
