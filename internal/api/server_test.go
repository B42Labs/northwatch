package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStaleHeaderMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	tests := []struct {
		name      string
		ready     func() bool
		path      string
		wantStale bool
	}{
		{"not ready marks api path", func() bool { return false }, "/api/v1/nb/Logical_Switch", true},
		{"ready leaves api path unmarked", func() bool { return true }, "/api/v1/nb/Logical_Switch", false},
		{"health path never marked", func() bool { return false }, "/healthz", false},
		{"cluster path attributed by proxy", func() bool { return false }, "/api/v1/clusters/prod/nb", false},
		{"nil predicate disables marker", nil, "/api/v1/nb/Logical_Switch", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := staleHeaderMiddleware(tc.ready, ok)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))

			got := w.Header().Get(staleHeader)
			if tc.wantStale {
				assert.Equal(t, "true", got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}
