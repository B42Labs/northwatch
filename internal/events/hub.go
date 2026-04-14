package events

import (
	"log"
	"sync"
)

const subscriberBufferSize = 256

// Subscriber represents a connected WebSocket client that receives events.
type Subscriber struct {
	C       chan Event
	id      uint64
	mu      sync.RWMutex
	filters []Filter
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
			log.Printf("events: dropping event for slow subscriber %d", s.id)
		}
	}
}

// SubscriberCount returns the current number of subscribers.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
