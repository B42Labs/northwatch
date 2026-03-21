package history

import (
	"context"
	"log"
	"time"

	"github.com/b42labs/northwatch/internal/events"
)

// TableSource defines how to fetch all rows for a given database table.
type TableSource struct {
	Database string
	Table    string
	ListFunc func(ctx context.Context) ([]map[string]any, error)
}

// Collector manages automatic snapshots, event persistence, and event pruning.
type Collector struct {
	store         *Store
	sources       []TableSource
	hub           *events.Hub
	interval      time.Duration
	retention     time.Duration
	eventMaxCount int64
}

// NewCollector creates a new history collector.
func NewCollector(store *Store, hub *events.Hub, sources []TableSource, interval, retention time.Duration) *Collector {
	return &Collector{
		store:     store,
		sources:   sources,
		hub:       hub,
		interval:  interval,
		retention: retention,
	}
}

// Store returns the underlying history store.
func (c *Collector) Store() *Store { return c.store }

// SetEventMaxCount sets the maximum number of events to retain.
// If maxCount is 0, no count-based pruning is performed.
func (c *Collector) SetEventMaxCount(maxCount int64) {
	c.eventMaxCount = maxCount
}

// TakeSnapshot captures the current state of all registered table sources.
func (c *Collector) TakeSnapshot(ctx context.Context, trigger, label string) (*SnapshotMeta, error) {
	var rows []SnapshotRow
	for _, src := range c.sources {
		data, err := src.ListFunc(ctx)
		if err != nil {
			log.Printf("history: snapshot source %s.%s failed: %v", src.Database, src.Table, err)
			continue
		}
		for _, d := range data {
			uuid, _ := d["_uuid"].(string)
			rows = append(rows, SnapshotRow{
				Database: src.Database,
				Table:    src.Table,
				UUID:     uuid,
				Data:     d,
			})
		}
	}
	return c.store.CreateSnapshot(ctx, trigger, label, rows)
}

// TakeSnapshotIfChanged captures a snapshot only if data has changed since
// the last snapshot. Returns nil without error if no changes are detected.
func (c *Collector) TakeSnapshotIfChanged(ctx context.Context, trigger, label string) (*SnapshotMeta, error) {
	var rows []SnapshotRow
	rowCounts := make(map[string]int)

	for _, src := range c.sources {
		data, err := src.ListFunc(ctx)
		if err != nil {
			log.Printf("history: snapshot source %s.%s failed: %v", src.Database, src.Table, err)
			continue
		}
		key := src.Database + "." + src.Table
		rowCounts[key] = len(data)
		for _, d := range data {
			uuid, _ := d["_uuid"].(string)
			rows = append(rows, SnapshotRow{
				Database: src.Database,
				Table:    src.Table,
				UUID:     uuid,
				Data:     d,
			})
		}
	}

	// Check if content hash differs from the latest snapshot
	currentHash := computeContentHash(rows)
	lastHash, err := c.store.LatestSnapshotContentHash(ctx)
	if err != nil {
		log.Printf("history: failed to get latest snapshot hash: %v", err)
		// Fall through and create the snapshot anyway
	} else if lastHash == currentHash {
		return nil, nil // no changes
	}

	return c.store.CreateSnapshot(ctx, trigger, label, rows)
}

// Start launches background goroutines for periodic snapshots, event persistence,
// and event pruning. Returns a cleanup function that stops all goroutines.
func (c *Collector) Start(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(ctx)

	// Goroutine 1: periodic snapshots (with deduplication)
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				meta, err := c.TakeSnapshotIfChanged(ctx, "auto", "")
				if err != nil {
					log.Printf("history: auto-snapshot failed: %v", err)
				} else if meta == nil {
					log.Printf("history: auto-snapshot skipped (no changes)")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Goroutine 2: event persistence
	sub := c.hub.Subscribe()
	sub.AddFilter(events.Filter{Database: "*", Tables: []string{"*"}})

	go func() {
		var batch []EventRecord
		flushTimer := time.NewTicker(100 * time.Millisecond)
		defer flushTimer.Stop()

		flush := func() {
			if len(batch) == 0 {
				return
			}
			if err := c.store.InsertEvents(ctx, batch); err != nil {
				log.Printf("history: event persistence failed: %v", err)
			}
			batch = batch[:0]
		}

		for {
			select {
			case evt, ok := <-sub.C:
				if !ok {
					flush()
					return
				}
				batch = append(batch, EventRecord{
					Timestamp: time.UnixMilli(evt.Ts),
					Type:      string(evt.Type),
					Database:  evt.Database,
					Table:     evt.Table,
					UUID:      evt.UUID,
					Row:       evt.Row,
					OldRow:    evt.OldRow,
				})
				if len(batch) >= 100 {
					flush()
				}
			case <-flushTimer.C:
				flush()
			case <-ctx.Done():
				flush()
				return
			}
		}
	}()

	// Goroutine 3: event pruning (time-based + count-based)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Time-based pruning
				if n, err := c.store.PruneEvents(ctx, c.retention); err != nil {
					log.Printf("history: event pruning failed: %v", err)
				} else if n > 0 {
					log.Printf("history: pruned %d old events", n)
				}

				// Count-based pruning
				if c.eventMaxCount > 0 {
					if n, err := c.store.PruneEventsByCount(ctx, c.eventMaxCount); err != nil {
						log.Printf("history: event count pruning failed: %v", err)
					} else if n > 0 {
						log.Printf("history: pruned %d events (count limit %d)", n, c.eventMaxCount)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return func() {
		cancel()
		c.hub.Unsubscribe(sub)
	}
}
