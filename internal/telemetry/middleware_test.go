package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteLabel(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		expect  string
	}{
		{"method and path", "GET /api/v1/nb/logical-switches", "/api/v1/nb/logical-switches"},
		{"wildcard segment", "GET /api/v1/nb/logical-switches/{uuid}", "/api/v1/nb/logical-switches/{uuid}"},
		{"no method", "/healthz", "/healthz"},
		{"unmatched route", "", unmatchedLabel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, routeLabel(tt.pattern))
		})
	}
}

// pathLabels returns the observed path label values of the request counter,
// mapped to their counts.
func pathLabels(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)

	out := map[string]float64{}
	for _, f := range families {
		if f.GetName() != "northwatch_http_requests_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "path" {
					out[lp.GetValue()] += m.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}

// testMux mirrors the shape of the real routes: a literal list route and a
// wildcard detail route.
func testMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nb/logical-switches", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/v1/nb/logical-switches/{uuid}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func TestMiddleware_Wrap(t *testing.T) {
	registry := prometheus.NewRegistry()
	handler := NewMiddleware(registry).Wrap(testMux())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/nb/logical-switches", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "ok", w.Body.String())

	assert.Equal(t, map[string]float64{"/api/v1/nb/logical-switches": 1}, pathLabels(t, registry))

	families, err := registry.Gather()
	require.NoError(t, err)
	var durationSeen bool
	for _, f := range families {
		if f.GetName() == "northwatch_http_request_duration_seconds" {
			durationSeen = true
		}
	}
	assert.True(t, durationSeen)
}

// TestMiddleware_LabelsByRoutePattern is the cardinality bound: distinct UUIDs
// collapse onto the route pattern that matched them, not their raw paths.
func TestMiddleware_LabelsByRoutePattern(t *testing.T) {
	registry := prometheus.NewRegistry()
	handler := NewMiddleware(registry).Wrap(testMux())

	for _, uuid := range []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"12345678-1234-1234-1234-123456789abc",
		"not-even-a-uuid",
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
			"/api/v1/nb/logical-switches/"+uuid, nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, map[string]float64{"/api/v1/nb/logical-switches/{uuid}": 3}, pathLabels(t, registry))
}

// TestMiddleware_UnmatchedRoutesCollapse is the reason the change exists: a scan
// of made-up URLs used to mint one time series per request. They must all land
// on a single label.
func TestMiddleware_UnmatchedRoutesCollapse(t *testing.T) {
	registry := prometheus.NewRegistry()
	handler := NewMiddleware(registry).Wrap(testMux())

	for i := range 50 {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
			fmt.Sprintf("/api/v1/does-not-exist-%d", i), nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, map[string]float64{unmatchedLabel: 50}, pathLabels(t, registry))
}

func TestResponseWriter_Flush(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMiddleware(registry)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := m.Wrap(inner)

	// httptest.ResponseRecorder implements Flusher, so this should not panic
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, w.Flushed)
}
