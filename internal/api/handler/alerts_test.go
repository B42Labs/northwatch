package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
}
