package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
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
			slog.Error("marshaling alert webhook payload failed", "err", err)
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
		slog.Error("creating alert webhook request failed", "url", url, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "northwatch-alertmanager")

	resp, err := w.client.Do(req)
	if err != nil {
		slog.Error("sending alert webhook failed", "url", url, "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		slog.Warn("alert webhook returned non-2xx status", "url", url, "status", resp.StatusCode)
	}
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
