package alert

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookNotifier_Notify(t *testing.T) {
	var mu sync.Mutex
	var received []WebhookPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "northwatch-alertmanager", r.Header.Get("User-Agent"))

		var payload WebhookPayload
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))

		mu.Lock()
		received = append(received, payload)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewWebhookNotifier([]string{srv.URL})

	alerts := []Alert{
		{
			Rule:     "test_alert",
			Severity: SeverityCritical,
			State:    StateFiring,
			Message:  "something is wrong",
			Labels:   map[string]string{"host": "node1"},
		},
		{
			Rule:     "test_resolved",
			Severity: SeverityWarning,
			State:    StateResolved,
			Message:  "resolved now",
		},
	}

	notifier.Notify(context.Background(), alerts)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, received, 2)

	// Check firing payload
	var firingPayload, resolvedPayload WebhookPayload
	for _, p := range received {
		if p.Status == "firing" {
			firingPayload = p
		} else {
			resolvedPayload = p
		}
	}

	assert.Equal(t, "firing", firingPayload.Status)
	require.Len(t, firingPayload.Alerts, 1)
	assert.Equal(t, "test_alert", firingPayload.Alerts[0].Rule)

	assert.Equal(t, "resolved", resolvedPayload.Status)
	require.Len(t, resolvedPayload.Alerts, 1)
	assert.Equal(t, "test_resolved", resolvedPayload.Alerts[0].Rule)
}

func TestWebhookNotifier_EmptyAlerts(t *testing.T) {
	notifier := NewWebhookNotifier([]string{"http://should-not-be-called"})
	// Should not panic or make requests
	notifier.Notify(context.Background(), nil)
	notifier.Notify(context.Background(), []Alert{})
}

func TestWebhookNotifier_NoURLs(t *testing.T) {
	notifier := NewWebhookNotifier(nil)
	notifier.Notify(context.Background(), []Alert{
		{Rule: "test", State: StateFiring},
	})
}

func TestWebhookNotifier_URLs(t *testing.T) {
	n := NewWebhookNotifier([]string{"http://a", "http://b"})
	urls := n.URLs()
	assert.Equal(t, []string{"http://a", "http://b"}, urls)

	// Modifying returned slice shouldn't affect notifier
	urls[0] = "http://modified"
	assert.Equal(t, "http://a", n.URLs()[0])
}

func TestWebhookNotifier_URLsNil(t *testing.T) {
	var n *WebhookNotifier
	assert.Nil(t, n.URLs())
}

func TestParseWebhookURLs(t *testing.T) {
	tests := []struct {
		input  string
		expect []string
	}{
		{"", nil},
		{"http://a", []string{"http://a"}},
		{"http://a,http://b", []string{"http://a", "http://b"}},
		{"http://a , http://b , http://c", []string{"http://a", "http://b", "http://c"}},
		{" , , ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseWebhookURLs(tt.input)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestWebhookNotifier_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	notifier := NewWebhookNotifier([]string{srv.URL})

	// Should not panic on server errors — just logs
	notifier.Notify(context.Background(), []Alert{
		{Rule: "test", State: StateFiring, Severity: SeverityWarning, Message: "test"},
	})
}

func TestFormatAlertSummary(t *testing.T) {
	a := Alert{
		Rule:     "port_down",
		Severity: SeverityWarning,
		State:    StateFiring,
		Message:  "Port sw1-p1 is down",
	}
	summary := FormatAlertSummary(a)
	assert.Equal(t, "[warning] port_down: Port sw1-p1 is down (state=firing)", summary)
}
