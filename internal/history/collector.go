package history

import (
	"context"
	"log/slog"
	"sync"
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
	paused        func() bool
	wg            sync.WaitGroup
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

// SetPauseCheck installs a predicate consulted before auto-snapshots and event
// persistence. While it returns true (e.g. a snapshot session has suspended the
// live monitors and purged the caches), the collector skips auto-snapshots and
// drops incoming events, so it neither records empty "auto" snapshots nor floods
// history with the re-population events emitted when monitors resume.
func (c *Collector) SetPauseCheck(fn func() bool) { c.paused = fn }

// isPaused reports whether the collector is currently paused.
func (c *Collector) isPaused() bool {
	return c.paused != nil && c.paused()
}

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
			slog.Error("history snapshot source failed", "database", src.Database, "table", src.Table, "err", err)
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
	// While paused (a snapshot session has suspended the live monitors and
	// purged the caches), the sources would read as empty and record a bogus
	// 100%-drop "auto" snapshot. Skip instead.
	if c.isPaused() {
		return nil, nil
	}

	var rows []SnapshotRow
	rowCounts := make(map[string]int)

	for _, src := range c.sources {
		data, err := src.ListFunc(ctx)
		if err != nil {
			slog.Error("history snapshot source failed", "database", src.Database, "table", src.Table, "err", err)
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
		slog.Error("getting latest snapshot hash failed", "err", err)
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
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				meta, err := c.TakeSnapshotIfChanged(ctx, "auto", "")
				if err != nil {
					slog.Error("history auto-snapshot failed", "err", err)
				} else if meta == nil {
					slog.Debug("history auto-snapshot skipped (no changes)")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Goroutine 2: event persistence
	sub := c.hub.Subscribe()
	sub.AddFilter(events.Filter{Database: "*", Tables: []string{"*"}})

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		var batch []EventRecord
		flushTimer := time.NewTicker(100 * time.Millisecond)
		defer flushTimer.Stop()

		// flush persists the accumulated batch. fctx is a separate parameter so
		// the shutdown flush can run on a context detached from cancellation:
		// InsertEvents uses fctx for BeginTx/ExecContext, so flushing on the
		// already-cancelled ctx would drop the final batch.
		flush := func(fctx context.Context) {
			if len(batch) == 0 {
				return
			}
			if err := c.store.InsertEvents(fctx, batch); err != nil {
				slog.Error("history event persistence failed", "err", err)
			}
			batch = batch[:0]
		}

		for {
			select {
			case evt, ok := <-sub.C:
				if !ok {
					flush(context.WithoutCancel(ctx))
					return
				}
				// Drop events while paused: during a snapshot session the cache
				// purge and later re-population emit a flood of OnDelete/OnAdd
				// events that do not reflect real changes.
				if c.isPaused() {
					continue
				}
				batch = append(batch, eventRecord(evt))
				if len(batch) >= 100 {
					flush(ctx)
				}
			case <-flushTimer.C:
				flush(ctx)
			case <-ctx.Done():
				// Drain events still buffered in the subscriber channel so a
				// graceful stop persists everything published before it, then
				// flush once on a context detached from cancellation.
			drain:
				for {
					select {
					case evt, ok := <-sub.C:
						if !ok {
							break drain
						}
						if !c.isPaused() {
							batch = append(batch, eventRecord(evt))
						}
					default:
						break drain
					}
				}
				flush(context.WithoutCancel(ctx))
				return
			}
		}
	}()

	// Goroutine 3: event pruning (time-based + count-based)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Time-based pruning
				if n, err := c.store.PruneEvents(ctx, c.retention); err != nil {
					slog.Error("history event pruning failed", "err", err)
				} else if n > 0 {
					slog.Info("history pruned old events", "count", n)
				}

				// Count-based pruning
				if c.eventMaxCount > 0 {
					if n, err := c.store.PruneEventsByCount(ctx, c.eventMaxCount); err != nil {
						slog.Error("history event count pruning failed", "err", err)
					} else if n > 0 {
						slog.Info("history pruned events over count limit", "count", n, "limit", c.eventMaxCount)
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
		// Wait for all three goroutines to return — in particular the event
		// persistence goroutine's final flush — before the caller closes the
		// store, so a shutdown flush cannot race Store.Close().
		c.wg.Wait()
	}
}

// eventRecord converts a hub event into the persisted EventRecord shape.
func eventRecord(evt events.Event) EventRecord {
	return EventRecord{
		Timestamp: time.UnixMilli(evt.Ts),
		Type:      string(evt.Type),
		Database:  evt.Database,
		Table:     evt.Table,
		UUID:      evt.UUID,
		Row:       evt.Row,
		OldRow:    evt.OldRow,
	}
}
