package alert

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/b42labs/northwatch/internal/events"
)

const resolvedAlertTTL = 5 * time.Minute

// Engine evaluates alert rules on a periodic interval and tracks active alerts.
type Engine struct {
	mu       sync.RWMutex
	rules    []Rule
	disabled map[string]bool   // rule names that are disabled
	active   map[string]Alert  // keyed by fingerprint
	silences map[string]Silence // keyed by ID
	hub      *events.Hub
	interval time.Duration
	notifier NotifierFunc
}

// NewEngine creates a new alert engine.
func NewEngine(hub *events.Hub, interval time.Duration) *Engine {
	return &Engine{
		active:   make(map[string]Alert),
		disabled: make(map[string]bool),
		silences: make(map[string]Silence),
		hub:      hub,
		interval: interval,
	}
}

// SetNotifier configures a function that is called when alerts fire or resolve.
func (e *Engine) SetNotifier(fn NotifierFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.notifier = fn
}

// RegisterRule adds a rule to the engine.
func (e *Engine) RegisterRule(r Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, r)
}

// SetRuleEnabled enables or disables a rule by name.
func (e *Engine) SetRuleEnabled(name string, enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	found := false
	for _, r := range e.rules {
		if r.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("rule %q: not found", name)
	}

	if enabled {
		delete(e.disabled, name)
	} else {
		e.disabled[name] = true
	}
	return nil
}

// AddSilence registers a silence and returns its ID.
func (e *Engine) AddSilence(s Silence) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if s.ID == "" {
		s.ID = fmt.Sprintf("silence-%d", time.Now().UnixNano())
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	e.silences[s.ID] = s
	return s.ID
}

// RemoveSilence removes a silence by ID. Returns an error if not found.
func (e *Engine) RemoveSilence(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.silences[id]; !ok {
		return fmt.Errorf("silence %q: not found", id)
	}
	delete(e.silences, id)
	return nil
}

// ListSilences returns all active (non-expired) silences.
func (e *Engine) ListSilences() []Silence {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()
	var result []Silence
	for _, s := range e.silences {
		if !s.IsExpired(now) {
			result = append(result, s)
		}
	}
	return result
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
	disabled := make(map[string]bool, len(e.disabled))
	for k, v := range e.disabled {
		disabled[k] = v
	}
	e.mu.RUnlock()

	now := time.Now()
	seen := map[string]bool{}
	var notifications []Alert

	for _, rule := range rules {
		if disabled[rule.Name] {
			continue
		}

		alerts := rule.Check(ctx)
		for _, a := range alerts {
			fp := a.fingerprint()
			seen[fp] = true

			// Hold the write lock for the entire check-and-insert so two
			// concurrent evaluations cannot both insert the same alert.
			e.mu.Lock()
			_, exists := e.active[fp]
			if !exists {
				a.State = StateFiring
				a.FiredAt = now
				e.active[fp] = a
			}
			e.mu.Unlock()

			if !exists {
				e.publishEvent(a, events.EventInsert)
				notifications = append(notifications, a)
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
			notifications = append(notifications, a)
		}
	}

	// Clean up resolved alerts older than the TTL
	for fp, a := range e.active {
		if a.State == StateResolved && a.ResolvedAt != nil && now.Sub(*a.ResolvedAt) > resolvedAlertTTL {
			delete(e.active, fp)
		}
	}

	// Clean up expired silences
	for id, s := range e.silences {
		if s.IsExpired(now) {
			delete(e.silences, id)
		}
	}

	notifier := e.notifier
	e.mu.Unlock()

	// Send webhook notifications outside the lock
	if notifier != nil && len(notifications) > 0 {
		notifier(ctx, notifications)
	}
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

// isSilenced checks whether an alert is suppressed by any active silence.
func (e *Engine) isSilenced(a Alert) bool {
	// Caller must hold e.mu (at least RLock)
	now := time.Now()
	for _, s := range e.silences {
		if !s.IsExpired(now) && s.Matches(a) {
			return true
		}
	}
	return false
}

// ActiveAlerts returns all currently firing alerts that are not silenced.
func (e *Engine) ActiveAlerts() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	alerts := make([]Alert, 0, len(e.active))
	for _, a := range e.active {
		if a.State == StateFiring && !e.isSilenced(a) {
			alerts = append(alerts, a)
		}
	}
	return alerts
}

// AllAlerts returns all currently firing alerts including silenced ones,
// with a "silenced" field set accordingly.
func (e *Engine) AllAlerts() []AlertWithStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var alerts []AlertWithStatus
	for _, a := range e.active {
		if a.State == StateFiring {
			alerts = append(alerts, AlertWithStatus{
				Alert:    a,
				Silenced: e.isSilenced(a),
			})
		}
	}
	return alerts
}

// AlertWithStatus wraps an Alert with additional status information.
type AlertWithStatus struct {
	Alert
	Silenced bool `json:"silenced"`
}

// Rules returns summaries of all registered rules including enabled state.
func (e *Engine) Rules() []RuleSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()
	summaries := make([]RuleSummary, len(e.rules))
	for i, r := range e.rules {
		summaries[i] = RuleSummary{
			Name:        r.Name,
			Description: r.Description,
			Severity:    r.Severity,
			Enabled:     !e.disabled[r.Name],
		}
	}
	return summaries
}
