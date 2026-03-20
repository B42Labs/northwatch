package alert

import (
	"context"
	"sync"
	"time"

	"github.com/b42labs/northwatch/internal/events"
)

const resolvedAlertTTL = 5 * time.Minute

// Engine evaluates alert rules on a periodic interval and tracks active alerts.
type Engine struct {
	mu       sync.RWMutex
	rules    []Rule
	active   map[string]Alert // keyed by fingerprint
	hub      *events.Hub
	interval time.Duration
}

// NewEngine creates a new alert engine.
func NewEngine(hub *events.Hub, interval time.Duration) *Engine {
	return &Engine{
		active:   make(map[string]Alert),
		hub:      hub,
		interval: interval,
	}
}

// RegisterRule adds a rule to the engine.
func (e *Engine) RegisterRule(r Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, r)
}

// Start begins the evaluation loop. Returns a stop function.
func (e *Engine) Start(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.evaluate(ctx)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func (e *Engine) evaluate(ctx context.Context) {
	e.mu.RLock()
	rules := make([]Rule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	now := time.Now()
	seen := map[string]bool{}

	for _, rule := range rules {
		alerts := rule.Check(ctx)
		for _, a := range alerts {
			fp := a.fingerprint()
			seen[fp] = true

			e.mu.RLock()
			_, exists := e.active[fp]
			e.mu.RUnlock()

			if !exists {
				a.State = StateFiring
				a.FiredAt = now

				e.mu.Lock()
				e.active[fp] = a
				e.mu.Unlock()

				e.publishEvent(a, events.EventInsert)
			}
		}
	}

	// Resolve alerts that are no longer firing
	e.mu.Lock()
	for fp, a := range e.active {
		if a.State == StateFiring && !seen[fp] {
			a.State = StateResolved
			resolved := now
			a.ResolvedAt = &resolved
			e.active[fp] = a

			e.publishEvent(a, events.EventUpdate)
		}
	}

	// Clean up resolved alerts older than the TTL
	for fp, a := range e.active {
		if a.State == StateResolved && a.ResolvedAt != nil && now.Sub(*a.ResolvedAt) > resolvedAlertTTL {
			delete(e.active, fp)
		}
	}
	e.mu.Unlock()
}

func (e *Engine) publishEvent(a Alert, eventType events.EventType) {
	if e.hub == nil {
		return
	}
	row := map[string]any{
		"rule":     a.Rule,
		"severity": string(a.Severity),
		"state":    string(a.State),
		"message":  a.Message,
		"labels":   a.Labels,
	}
	e.hub.Publish(events.NewEvent(eventType, "alert", a.Rule, a.fingerprint(), row, nil))
}

// ActiveAlerts returns all currently firing alerts.
func (e *Engine) ActiveAlerts() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	alerts := make([]Alert, 0, len(e.active))
	for _, a := range e.active {
		if a.State == StateFiring {
			alerts = append(alerts, a)
		}
	}
	return alerts
}

// Rules returns summaries of all registered rules.
func (e *Engine) Rules() []RuleSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()
	summaries := make([]RuleSummary, len(e.rules))
	for i, r := range e.rules {
		summaries[i] = RuleSummary{
			Name:        r.Name,
			Description: r.Description,
			Severity:    r.Severity,
		}
	}
	return summaries
}
