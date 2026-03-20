package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
