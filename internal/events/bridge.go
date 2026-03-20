package events

import (
	"github.com/b42labs/northwatch/internal/api"
	"github.com/ovn-kubernetes/libovsdb/model"
)

// Bridge implements cache.EventHandler and converts OVSDB model events
// into Event structs, publishing them to the Hub.
type Bridge struct {
	hub      *Hub
	database string // "nb" or "sb"
}

// NewBridge creates a bridge that publishes events for the given database.
func NewBridge(hub *Hub, database string) *Bridge {
	return &Bridge{hub: hub, database: database}
}

// OnAdd is called when a row is inserted into the cache.
func (b *Bridge) OnAdd(table string, m model.Model) {
	row := api.ModelToMap(m)
	uuid := extractUUID(row)
	b.hub.Publish(NewEvent(EventInsert, b.database, table, uuid, row, nil))
}

// OnUpdate is called when a row is updated in the cache.
func (b *Bridge) OnUpdate(table string, old model.Model, new model.Model) {
	row := api.ModelToMap(new)
	oldRow := api.ModelToMap(old)
	uuid := extractUUID(row)
	b.hub.Publish(NewEvent(EventUpdate, b.database, table, uuid, row, oldRow))
}

// OnDelete is called when a row is deleted from the cache.
func (b *Bridge) OnDelete(table string, m model.Model) {
	row := api.ModelToMap(m)
	uuid := extractUUID(row)
	b.hub.Publish(NewEvent(EventDelete, b.database, table, uuid, row, nil))
}

func extractUUID(row map[string]any) string {
	if v, ok := row["_uuid"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
