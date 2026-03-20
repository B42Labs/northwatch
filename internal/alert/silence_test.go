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

func TestSilence_Matches(t *testing.T) {
	tests := []struct {
		name    string
		silence Silence
		alert   Alert
		match   bool
	}{
		{
			name:    "rule match",
			silence: Silence{Rule: "port_down"},
			alert:   Alert{Rule: "port_down"},
			match:   true,
		},
		{
			name:    "rule mismatch",
			silence: Silence{Rule: "port_down"},
			alert:   Alert{Rule: "bfd_down"},
			match:   false,
		},
		{
			name:    "empty rule matches any",
			silence: Silence{},
			alert:   Alert{Rule: "anything"},
			match:   true,
		},
		{
			name:    "label match",
			silence: Silence{Matchers: map[string]string{"chassis": "ch-1"}},
			alert:   Alert{Rule: "test", Labels: map[string]string{"chassis": "ch-1", "host": "h1"}},
			match:   true,
		},
		{
			name:    "label mismatch",
			silence: Silence{Matchers: map[string]string{"chassis": "ch-1"}},
			alert:   Alert{Rule: "test", Labels: map[string]string{"chassis": "ch-2"}},
			match:   false,
		},
		{
			name:    "rule and label match",
			silence: Silence{Rule: "port_down", Matchers: map[string]string{"port": "p1"}},
			alert:   Alert{Rule: "port_down", Labels: map[string]string{"port": "p1"}},
			match:   true,
		},
		{
			name:    "rule match but label mismatch",
			silence: Silence{Rule: "port_down", Matchers: map[string]string{"port": "p1"}},
			alert:   Alert{Rule: "port_down", Labels: map[string]string{"port": "p2"}},
			match:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.match, tt.silence.Matches(tt.alert))
		})
	}
}

func TestSilence_IsExpired(t *testing.T) {
	now := time.Now()
	s := Silence{ExpiresAt: now.Add(-time.Minute)}
	assert.True(t, s.IsExpired(now))

	s2 := Silence{ExpiresAt: now.Add(time.Hour)}
	assert.False(t, s2.IsExpired(now))
}

func TestEngine_Silences(t *testing.T) {
	engine := NewEngine(nil, 30*time.Second)

	// Add a silence
	s := Silence{
		Rule:      "port_down",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Comment:   "maintenance window",
	}
	id := engine.AddSilence(s)
	assert.NotEmpty(t, id)

	// List silences
	silences := engine.ListSilences()
	require.Len(t, silences, 1)
	assert.Equal(t, id, silences[0].ID)
	assert.Equal(t, "port_down", silences[0].Rule)
	assert.Equal(t, "maintenance window", silences[0].Comment)

	// Remove silence
	err := engine.RemoveSilence(id)
	require.NoError(t, err)

	assert.Empty(t, engine.ListSilences())

	// Remove non-existent
	err = engine.RemoveSilence("nonexistent")
	assert.Error(t, err)
}

func TestEngine_SilencedAlertsNotReturned(t *testing.T) {
	hub := events.NewHub()
	var firing atomic.Bool
	firing.Store(true)

	engine := NewEngine(hub, 50*time.Millisecond)
	engine.RegisterRule(Rule{
		Name:     "silenced_rule",
		Severity: SeverityWarning,
		Check: func(ctx context.Context) []Alert {
			if firing.Load() {
				return []Alert{{
					Rule:     "silenced_rule",
					Severity: SeverityWarning,
					Message:  "test",
					Labels:   map[string]string{},
				}}
			}
			return nil
		},
	})

	// Add silence before alert fires
	engine.AddSilence(Silence{
		Rule:      "silenced_rule",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	stop := engine.Start(context.Background())
	defer stop()

	// Wait for evaluation
	time.Sleep(200 * time.Millisecond)

	// ActiveAlerts should not include silenced alerts
	active := engine.ActiveAlerts()
	assert.Empty(t, active)

	// AllAlerts should include them with Silenced=true
	all := engine.AllAlerts()
	require.Len(t, all, 1)
	assert.True(t, all[0].Silenced)
	assert.Equal(t, "silenced_rule", all[0].Rule)
}

func TestEngine_ExpiredSilencesCleanedUp(t *testing.T) {
	engine := NewEngine(nil, 30*time.Second)

	// Add an already-expired silence
	engine.AddSilence(Silence{
		Rule:      "test",
		ExpiresAt: time.Now().Add(-time.Minute),
	})

	// Expired silences should not appear in ListSilences
	assert.Empty(t, engine.ListSilences())
}

func TestEngine_SetRuleEnabled(t *testing.T) {
	engine := NewEngine(nil, 30*time.Second)
	engine.RegisterRule(Rule{
		Name:        "test_rule",
		Description: "A test rule",
		Severity:    SeverityWarning,
		Check:       func(ctx context.Context) []Alert { return nil },
	})

	// Initially enabled
	rules := engine.Rules()
	require.Len(t, rules, 1)
	assert.True(t, rules[0].Enabled)

	// Disable
	err := engine.SetRuleEnabled("test_rule", false)
	require.NoError(t, err)

	rules = engine.Rules()
	assert.False(t, rules[0].Enabled)

	// Re-enable
	err = engine.SetRuleEnabled("test_rule", true)
	require.NoError(t, err)

	rules = engine.Rules()
	assert.True(t, rules[0].Enabled)

	// Non-existent rule
	err = engine.SetRuleEnabled("nonexistent", false)
	assert.Error(t, err)
}

func TestEngine_DisabledRuleNotEvaluated(t *testing.T) {
	hub := events.NewHub()
	engine := NewEngine(hub, 50*time.Millisecond)

	var callCount atomic.Int32
	engine.RegisterRule(Rule{
		Name:     "disabled_rule",
		Severity: SeverityWarning,
		Check: func(ctx context.Context) []Alert {
			callCount.Add(1)
			return []Alert{{
				Rule:     "disabled_rule",
				Severity: SeverityWarning,
				Message:  "should not fire",
				Labels:   map[string]string{},
			}}
		},
	})

	// Disable before starting
	require.NoError(t, engine.SetRuleEnabled("disabled_rule", false))

	stop := engine.Start(context.Background())
	defer stop()

	time.Sleep(200 * time.Millisecond)

	// Rule should not have been called
	assert.Equal(t, int32(0), callCount.Load())
	assert.Empty(t, engine.ActiveAlerts())
}
