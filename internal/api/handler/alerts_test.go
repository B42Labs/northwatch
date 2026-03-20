package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/alert"
)

func TestAlerts_EmptyList(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/alerts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body []any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Empty(t, body)
}

func TestAlerts_Rules(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)
	engine.RegisterRule(alert.Rule{
		Name:        "test_rule",
		Description: "A test rule",
		Severity:    alert.SeverityWarning,
		Check:       func(ctx context.Context) []alert.Alert { return nil },
	})

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/alerts/rules", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body, 1)
	assert.Equal(t, "test_rule", body[0]["name"])
	assert.Equal(t, "warning", body[0]["severity"])
	assert.Equal(t, true, body[0]["enabled"])
}

func TestAlerts_SetRuleEnabled(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)
	engine.RegisterRule(alert.Rule{
		Name:     "my_rule",
		Severity: alert.SeverityWarning,
		Check:    func(ctx context.Context) []alert.Alert { return nil },
	})

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	// Disable rule
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/alerts/rules/my_rule",
		strings.NewReader(`{"enabled":false}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["enabled"])

	// Verify via rules list
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/alerts/rules", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var rules []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rules))
	assert.Equal(t, false, rules[0]["enabled"])
}

func TestAlerts_SetRuleEnabled_NotFound(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/alerts/rules/nonexistent",
		strings.NewReader(`{"enabled":false}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAlerts_SetRuleEnabled_InvalidBody(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)
	engine.RegisterRule(alert.Rule{Name: "r1", Check: func(ctx context.Context) []alert.Alert { return nil }})

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/alerts/rules/r1",
		strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAlerts_Silences_CRUD(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	// List empty silences
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/alerts/silences", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var silences []alert.Silence
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &silences))
	assert.Empty(t, silences)

	// Create silence
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/alerts/silences",
		strings.NewReader(`{"rule":"port_down","duration":"2h","comment":"maintenance"}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var created alert.Silence
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "port_down", created.Rule)
	assert.Equal(t, "maintenance", created.Comment)

	// List silences — should have 1
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/alerts/silences", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &silences))
	assert.Len(t, silences, 1)

	// Delete silence
	req = httptest.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/alerts/silences/"+created.ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// List silences — should be empty again
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/alerts/silences", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &silences))
	assert.Empty(t, silences)
}

func TestAlerts_CreateSilence_MissingFields(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	// Neither rule nor matchers provided
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/alerts/silences",
		strings.NewReader(`{"duration":"1h"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAlerts_CreateSilence_InvalidDuration(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/alerts/silences",
		strings.NewReader(`{"rule":"test","duration":"not-a-duration"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAlerts_CreateSilence_DefaultDuration(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/alerts/silences",
		strings.NewReader(`{"rule":"test"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var created alert.Silence
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	// Default duration is 1 hour
	assert.WithinDuration(t, time.Now().Add(1*time.Hour), created.ExpiresAt, 5*time.Second)
}

func TestAlerts_CreateSilence_WithMatchers(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/alerts/silences",
		strings.NewReader(`{"matchers":{"chassis":"ch-1"},"duration":"30m"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAlerts_DeleteSilence_NotFound(t *testing.T) {
	engine := alert.NewEngine(nil, 30*time.Second)

	mux := http.NewServeMux()
	RegisterAlerts(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/alerts/silences/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
