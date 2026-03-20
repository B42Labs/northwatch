package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"no uuid", "/api/v1/nb/Logical_Switch", "/api/v1/nb/Logical_Switch"},
		{"single uuid", "/api/v1/nb/Logical_Switch/550e8400-e29b-41d4-a716-446655440000", "/api/v1/nb/Logical_Switch/{uuid}"},
		{"two uuids", "/api/v1/correlated/550e8400-e29b-41d4-a716-446655440000/ports/12345678-1234-1234-1234-123456789abc", "/api/v1/correlated/{uuid}/ports/{uuid}"},
		{"uppercase not matched", "/api/v1/test/550E8400-E29B-41D4-A716-446655440000", "/api/v1/test/550E8400-E29B-41D4-A716-446655440000"},
		{"empty", "", ""},
		{"root", "/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, normalizePath(tt.input))
		})
	}
}

func TestMiddleware_Wrap(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMiddleware(registry)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	handler := m.Wrap(inner)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "ok", w.Body.String())

	// Verify metrics were recorded
	families, err := registry.Gather()
	require.NoError(t, err)

	metrics := map[string]*dto.MetricFamily{}
	for _, f := range families {
		metrics[f.GetName()] = f
	}

	totalFam := metrics["northwatch_http_requests_total"]
	require.NotNil(t, totalFam)
	assert.Len(t, totalFam.GetMetric(), 1)

	durationFam := metrics["northwatch_http_request_duration_seconds"]
	require.NotNil(t, durationFam)
}

func TestMiddleware_UUIDNormalization(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMiddleware(registry)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.Wrap(inner)

	// Two requests with different UUIDs should be recorded under the same path label
	req1 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/nb/Logical_Switch/550e8400-e29b-41d4-a716-446655440000", nil)
	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/nb/Logical_Switch/12345678-1234-1234-1234-123456789abc", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req1)
	handler.ServeHTTP(httptest.NewRecorder(), req2)

	families, err := registry.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() == "northwatch_http_requests_total" {
			require.Len(t, f.GetMetric(), 1, "both requests should map to the same label set")
			for _, lp := range f.GetMetric()[0].GetLabel() {
				if lp.GetName() == "path" {
					assert.Equal(t, "/api/v1/nb/Logical_Switch/{uuid}", lp.GetValue())
				}
			}
		}
	}
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
