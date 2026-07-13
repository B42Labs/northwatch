package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "world", body["hello"])
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, "not found")

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "not found", body["error"])
}

func TestWriteInternalError(t *testing.T) {
	var logged bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/snapshots", nil)
	WriteInternalError(w, r, errors.New(`opening /var/lib/northwatch/history.db: permission denied`))

	require.Equal(t, http.StatusInternalServerError, w.Code)

	// The client learns nothing about the internals...
	assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), "history.db")

	// ...but the cause is on the server, with the request that triggered it.
	assert.Contains(t, logged.String(), "permission denied")
	assert.Contains(t, logged.String(), "/api/v1/snapshots")
}
