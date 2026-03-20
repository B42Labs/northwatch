package flowdiff

import (
	"log"

	"github.com/b42labs/northwatch/internal/events"
)

// StartCollector subscribes to the event hub for LogicalFlow changes and
// records them in the store. Returns a cleanup function.
func StartCollector(hub *events.Hub, store *Store) func() {
	sub := hub.Subscribe()
	sub.AddFilter(events.Filter{
		Database: "sb",
		Tables:   []string{"Logical_Flow"},
	})

	done := make(chan struct{})
	go func() {
		for {
			select {
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				change := eventToFlowChange(evt)
				store.Add(change)
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
		hub.Unsubscribe(sub)
	}
}

func eventToFlowChange(evt events.Event) FlowChange {
	change := FlowChange{
		Timestamp: evt.Ts,
		Type:      string(evt.Type),
		UUID:      evt.UUID,
		NewRow:    evt.Row,
		OldRow:    evt.OldRow,
	}

	// Extract datapath from row or old_row
	change.Datapath = extractDatapath(evt.Row)
	if change.Datapath == "" {
		change.Datapath = extractDatapath(evt.OldRow)
	}

	return change
}

func extractDatapath(row map[string]any) string {
	if row == nil {
		return ""
	}
	if dp, ok := row["logical_datapath"]; ok {
		switch v := dp.(type) {
		case string:
			return v
		case *string:
			if v != nil {
				return *v
			}
		default:
			log.Printf("flowdiff: unexpected logical_datapath type: %T", dp)
		}
	}
	return ""
}
