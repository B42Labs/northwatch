package events

import "time"

// EventType represents the type of OVSDB cache change.
type EventType string

const (
	EventInsert EventType = "insert"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)

// Event represents a single OVSDB change pushed to WebSocket subscribers.
type Event struct {
	Type     EventType      `json:"type"`
	Database string         `json:"database"`
	Table    string         `json:"table"`
	UUID     string         `json:"uuid"`
	Row      map[string]any `json:"row,omitempty"`
	OldRow   map[string]any `json:"old_row,omitempty"`
	Ts       int64          `json:"ts"`
}

// SubscribeMessage is sent by the WebSocket client to manage subscriptions.
type SubscribeMessage struct {
	Action   string   `json:"action"`
	Database string   `json:"database"`
	Tables   []string `json:"tables"`
}

// Filter determines which events a subscriber wants to receive.
type Filter struct {
	Database string   // "*" or "nb" or "sb"
	Tables   []string // ["*"] or specific table names
}

// Matches returns true if the event matches this filter.
func (f Filter) Matches(e Event) bool {
	if f.Database != "*" && f.Database != e.Database {
		return false
	}
	if len(f.Tables) == 0 {
		return false
	}
	if f.Tables[0] == "*" {
		return true
	}
	for _, t := range f.Tables {
		if t == e.Table {
			return true
		}
	}
	return false
}

// NewEvent creates an Event with the current timestamp.
func NewEvent(eventType EventType, database, table, uuid string, row, oldRow map[string]any) Event {
	return Event{
		Type:     eventType,
		Database: database,
		Table:    table,
		UUID:     uuid,
		Row:      row,
		OldRow:   oldRow,
		Ts:       time.Now().UnixMilli(),
	}
}
