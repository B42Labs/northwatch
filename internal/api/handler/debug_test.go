package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlePortDiagnostics_InvalidParams(t *testing.T) {
	// Both params are validated before the diagnoser is touched, so a nil
	// diagnoser is sufficient to exercise the 400 paths.
	tests := []struct {
		name string
		url  string
	}{
		{"invalid severity", "/api/v1/debug/port-diagnostics?severity=critical"},
		{"non-numeric limit", "/api/v1/debug/port-diagnostics?limit=abc"},
		{"negative limit", "/api/v1/debug/port-diagnostics?limit=-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handlePortDiagnostics(nil)
			req, err := http.NewRequestWithContext(context.Background(), "GET", tt.url, nil)
			require.NoError(t, err)
			w := httptest.NewRecorder()

			handler(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleConnectivity_MissingParams(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/api/v1/debug/connectivity"},
		{"missing dst", "/api/v1/debug/connectivity?src=abc"},
		{"missing src", "/api/v1/debug/connectivity?dst=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleConnectivity(nil) // checker not needed for param validation
			req, err := http.NewRequestWithContext(context.Background(), "GET", tt.url, nil)
			require.NoError(t, err)
			w := httptest.NewRecorder()

			handler(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}
