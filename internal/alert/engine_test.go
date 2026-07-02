package alert

import (
	"context"
	"errors"
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
		Check:       func(ctx context.Context) ([]Alert, error) { return nil, nil },
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
		Check: func(ctx context.Context) ([]Alert, error) {
			if firing.Load() {
				return []Alert{{
					Rule:     "test_alert",
					Severity: SeverityCritical,
					Message:  "something is wrong",
					Labels:   map[string]string{},
				}}, nil
			}
			return nil, nil
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

func TestEngine_PausedSkipsEvaluation(t *testing.T) {
	engine := NewEngine(events.NewHub(), time.Hour)
	var checked atomic.Bool
	engine.RegisterRule(Rule{
		Name:     "should_not_run",
		Severity: SeverityCritical,
		Check: func(ctx context.Context) ([]Alert, error) {
			checked.Store(true)
			return []Alert{{Rule: "should_not_run", Severity: SeverityCritical, Labels: map[string]string{}}}, nil
		},
	})
	engine.SetPauseCheck(func() bool { return true })

	engine.evaluate(context.Background())

	assert.False(t, checked.Load(), "rules must not be checked while paused")
	assert.Empty(t, engine.ActiveAlerts(), "no alerts should fire while paused")
}

func TestEngine_RuleErrorSkipsResolve(t *testing.T) {
	engine := NewEngine(events.NewHub(), time.Hour)
	var fire, errOut atomic.Bool
	fire.Store(true)
	engine.RegisterRule(Rule{
		Name:     "flaky",
		Severity: SeverityCritical,
		Check: func(ctx context.Context) ([]Alert, error) {
			if errOut.Load() {
				return nil, errors.New("list failed")
			}
			if fire.Load() {
				return []Alert{{Rule: "flaky", Severity: SeverityCritical, Labels: map[string]string{}}}, nil
			}
			return nil, nil
		},
	})

	// First eval fires the alert.
	engine.evaluate(context.Background())
	require.Len(t, engine.ActiveAlerts(), 1)

	// The rule now errors (e.g. an OVSDB reconnect blip): the active alert must
	// NOT be resolved.
	errOut.Store(true)
	engine.evaluate(context.Background())
	require.Len(t, engine.ActiveAlerts(), 1, "a rule read failure must not resolve its active alerts")

	// The rule recovers and genuinely reports no alerts: now it resolves.
	errOut.Store(false)
	fire.Store(false)
	engine.evaluate(context.Background())
	assert.Empty(t, engine.ActiveAlerts())
}

func TestEngine_SilencedAlertsDoNotNotify(t *testing.T) {
	hub := events.NewHub()
	sub := hub.Subscribe()
	sub.AddFilter(events.Filter{Database: "alert", Tables: []string{"*"}})

	// evaluate calls the notifier synchronously in this goroutine, so a plain
	// slice needs no synchronization.
	var notified []Alert
	engine := NewEngine(hub, time.Hour)
	engine.SetNotifier(func(ctx context.Context, alerts []Alert) { notified = append(notified, alerts...) })

	firingRule := func(name string) Rule {
		return Rule{Name: name, Severity: SeverityWarning, Check: func(ctx context.Context) ([]Alert, error) {
			return []Alert{{Rule: name, Severity: SeverityWarning, Labels: map[string]string{}}}, nil
		}}
	}
	engine.RegisterRule(firingRule("silenced"))
	engine.RegisterRule(firingRule("loud"))
	engine.AddSilence(Silence{Rule: "silenced", ExpiresAt: time.Now().Add(time.Hour)})

	engine.evaluate(context.Background())

	// The silenced alert must neither page nor publish; the loud one must do both.
	require.Len(t, notified, 1, "only the unsilenced alert may notify")
	assert.Equal(t, "loud", notified[0].Rule)

	select {
	case e := <-sub.C:
		assert.Equal(t, events.EventInsert, e.Type)
		assert.Equal(t, "loud", e.Table, "only the unsilenced alert may publish an event")
	case <-time.After(time.Second):
		t.Fatal("expected the unsilenced alert to publish a hub event")
	}
	select {
	case e := <-sub.C:
		t.Fatalf("silenced alert must not publish a hub event, got %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEngine_SilencedThenUnsilencedAlertPages(t *testing.T) {
	// evaluate calls the notifier synchronously in this goroutine, so a plain
	// slice needs no synchronization.
	var notified []Alert
	engine := NewEngine(events.NewHub(), time.Hour)
	engine.SetNotifier(func(ctx context.Context, alerts []Alert) { notified = append(notified, alerts...) })

	engine.RegisterRule(Rule{Name: "silenced_first", Severity: SeverityWarning,
		Check: func(ctx context.Context) ([]Alert, error) {
			return []Alert{{Rule: "silenced_first", Severity: SeverityWarning, Labels: map[string]string{}}}, nil
		}})
	silenceID := engine.AddSilence(Silence{Rule: "silenced_first", ExpiresAt: time.Now().Add(time.Hour)})

	// First tick: the alert fires while silenced — tracked but not paged.
	engine.evaluate(context.Background())
	require.Empty(t, notified, "an alert that first fires while silenced must not page")

	// Second tick, still silenced: still no page.
	engine.evaluate(context.Background())
	require.Empty(t, notified, "a silenced alert must stay unpaged while the silence holds")

	// The silence is lifted while the alert is still firing: it must page now.
	require.NoError(t, engine.RemoveSilence(silenceID))
	engine.evaluate(context.Background())
	require.Len(t, notified, 1, "an alert must page once its silence lifts, even though it fired earlier")
	assert.Equal(t, "silenced_first", notified[0].Rule)

	// A further tick must not re-page the already-paged alert.
	engine.evaluate(context.Background())
	require.Len(t, notified, 1, "an already-paged alert must not page again on subsequent ticks")
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
