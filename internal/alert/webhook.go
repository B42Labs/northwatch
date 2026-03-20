package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// WebhookNotifier sends alert state changes to configured webhook URLs.
type WebhookNotifier struct {
	urls   []string
	client *http.Client
}

// NewWebhookNotifier creates a notifier that POSTs alert payloads to the given URLs.
func NewWebhookNotifier(urls []string) *WebhookNotifier {
	return &WebhookNotifier{
		urls: urls,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// WebhookPayload is the JSON body sent to webhook endpoints.
type WebhookPayload struct {
	Status string  `json:"status"` // "firing" or "resolved"
	Alerts []Alert `json:"alerts"`
}

// Notify sends alert state changes to all configured webhook URLs.
func (w *WebhookNotifier) Notify(ctx context.Context, alerts []Alert) {
	if len(w.urls) == 0 || len(alerts) == 0 {
		return
	}

	// Group alerts by state
	var firing, resolved []Alert
	for _, a := range alerts {
		switch a.State {
		case StateFiring:
			firing = append(firing, a)
		case StateResolved:
			resolved = append(resolved, a)
		}
	}

	var payloads []WebhookPayload
	if len(firing) > 0 {
		payloads = append(payloads, WebhookPayload{Status: "firing", Alerts: firing})
	}
	if len(resolved) > 0 {
		payloads = append(payloads, WebhookPayload{Status: "resolved", Alerts: resolved})
	}

	for _, payload := range payloads {
		body, err := json.Marshal(payload)
		if err != nil {
			log.Printf("alert webhook: failed to marshal payload: %v", err)
			continue
		}
		for _, url := range w.urls {
			w.post(ctx, url, body)
		}
	}
}

func (w *WebhookNotifier) post(ctx context.Context, url string, body []byte) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert webhook: failed to create request for %s: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "northwatch-alertmanager")

	resp, err := w.client.Do(req)
	if err != nil {
		log.Printf("alert webhook: failed to send to %s: %v", url, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		log.Printf("alert webhook: %s returned status %d", url, resp.StatusCode)
	}
}

// URLs returns the configured webhook URLs.
func (w *WebhookNotifier) URLs() []string {
	if w == nil {
		return nil
	}
	result := make([]string, len(w.urls))
	copy(result, w.urls)
	return result
}

// SetClient allows overriding the HTTP client (useful for testing).
func (w *WebhookNotifier) SetClient(c *http.Client) {
	w.client = c
}

// parseWebhookURLs splits a comma-separated list of webhook URLs.
func ParseWebhookURLs(s string) []string {
	if s == "" {
		return nil
	}
	var urls []string
	for _, u := range splitTrimmed(s, ",") {
		if u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

func splitTrimmed(s, sep string) []string {
	var result []string
	for _, part := range bytes.Split([]byte(s), []byte(sep)) {
		trimmed := bytes.TrimSpace(part)
		if len(trimmed) > 0 {
			result = append(result, string(trimmed))
		}
	}
	return result
}

// NotifierFunc is the signature used by the engine to send notifications.
type NotifierFunc func(ctx context.Context, alerts []Alert)

// Notifier wraps WebhookNotifier.Notify as a NotifierFunc.
func (w *WebhookNotifier) Notifier() NotifierFunc {
	return func(ctx context.Context, alerts []Alert) {
		w.Notify(ctx, alerts)
	}
}

// FormatAlertSummary returns a human-readable one-line summary for logging.
func FormatAlertSummary(a Alert) string {
	return fmt.Sprintf("[%s] %s: %s (state=%s)", a.Severity, a.Rule, a.Message, a.State)
}
