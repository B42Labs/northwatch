package alert

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Severity represents the severity of an alert.
type Severity string

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// AlertState represents the current state of an alert.
type AlertState string

const (
	StateFiring   AlertState = "firing"
	StateResolved AlertState = "resolved"
)

// Rule defines a check that produces alerts. Check returns an error when it
// cannot determine the current state (e.g. an OVSDB list failed), which is
// distinct from "no alerts": the engine skips the resolve pass for a rule that
// errors so a transient read failure does not resolve then re-fire everything.
type Rule struct {
	Name        string
	Description string
	Severity    Severity
	Check       func(ctx context.Context) ([]Alert, error)
}

// RuleSummary is the JSON representation of a rule (without the Check function).
type RuleSummary struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
	Enabled     bool     `json:"enabled"`
}

// Alert represents a single alert instance.
type Alert struct {
	Rule       string            `json:"rule"`
	Severity   Severity          `json:"severity"`
	State      AlertState        `json:"state"`
	Message    string            `json:"message"`
	Labels     map[string]string `json:"labels,omitempty"`
	FiredAt    time.Time         `json:"fired_at"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`

	// notified records whether this active alert has already produced a page
	// (webhook + WS event). It stays false while the alert is suppressed by a
	// silence, so the page fires on the first non-silenced tick even when the
	// alert started firing earlier, while it was still silenced.
	notified bool
}

// fingerprint returns a unique key for deduplication based on rule name and labels.
func (a Alert) fingerprint() string {
	if len(a.Labels) == 0 {
		return a.Rule
	}
	keys := make([]string, 0, len(a.Labels))
	for k := range a.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, a.Rule)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, a.Labels[k]))
	}
	return strings.Join(parts, "/")
}
