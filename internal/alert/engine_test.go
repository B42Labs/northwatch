package alert

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_RegisterAndListRules(t *testing.T) {
	engine := NewEngine(events.NewHub(), 30*time.Second)
	engine.RegisterRule(Rule{
		Name:        "test_rule",
		Description: "A test rule",
		Severity:    SeverityWarning,
		Check:       func(ctx context.Context) []Alert { return nil },
	})

	rules := engine.Rules()
	require.Len(t, rules, 1)
	assert.Equal(t, "test_rule", rules[0].Name)
	assert.Equal(t, SeverityWarning, rules[0].Severity)
}

func TestEngine_EvaluateFiresAndResolves(t *testing.T) {
	hub := events.NewHub()
	sub := hub.Subscribe()
	sub.AddFilter(events.Filter{Database: "alert", Tables: []string{"*"}})

	var firing atomic.Bool
	firing.Store(true)
	engine := NewEngine(hub, 50*time.Millisecond)
	engine.RegisterRule(Rule{
		Name:        "test_alert",
		Description: "Fires when flag is true",
		Severity:    SeverityCritical,
		Check: func(ctx context.Context) []Alert {
			if firing.Load() {
				return []Alert{{
					Rule:     "test_alert",
					Severity: SeverityCritical,
					Message:  "something is wrong",
					Labels:   map[string]string{},
				}}
			}
			return nil
		},
	})

	stop := engine.Start(context.Background())
	defer stop()

	// Wait for the alert to fire
	var fired bool
	select {
	case e := <-sub.C:
		assert.Equal(t, events.EventInsert, e.Type)
		assert.Equal(t, "alert", e.Database)
		fired = true
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for alert event")
	}
	require.True(t, fired)

	// Verify active alerts
	active := engine.ActiveAlerts()
	require.Len(t, active, 1)
	assert.Equal(t, StateFiring, active[0].State)

	// Stop firing
	firing.Store(false)

	// Wait for resolution event
	select {
	case e := <-sub.C:
		assert.Equal(t, events.EventUpdate, e.Type)
		row, ok := e.Row["state"].(string)
		require.True(t, ok)
		assert.Equal(t, "resolved", row)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resolve event")
	}

	// No active alerts after resolution
	active = engine.ActiveAlerts()
	assert.Empty(t, active)
}

func TestEngine_ActiveAlertsEmpty(t *testing.T) {
	engine := NewEngine(nil, 30*time.Second)
	assert.Empty(t, engine.ActiveAlerts())
}

func TestAlert_Fingerprint(t *testing.T) {
	tests := []struct {
		name   string
		alert  Alert
		expect string
	}{
		{
			name:   "no labels",
			alert:  Alert{Rule: "test"},
			expect: "test",
		},
		{
			name:   "with labels",
			alert:  Alert{Rule: "test", Labels: map[string]string{"b": "2", "a": "1"}},
			expect: "test/a=1/b=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.alert.fingerprint())
		})
	}
}
