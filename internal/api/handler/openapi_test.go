package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/openapi"
)

func TestOpenAPIJSON(t *testing.T) {
	spec := openapi.BuildSpec()
	mux := http.NewServeMux()
	RegisterOpenAPI(mux, spec)

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var doc openapi.Document
	err := json.Unmarshal(rec.Body.Bytes(), &doc)
	require.NoError(t, err)
	assert.Equal(t, "3.1.0", doc.OpenAPI)
	assert.NotEmpty(t, doc.Paths)
}

func TestOpenAPIDocs(t *testing.T) {
	spec := openapi.BuildSpec()
	mux := http.NewServeMux()
	RegisterOpenAPI(mux, spec)

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/docs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html"))
	assert.Contains(t, rec.Body.String(), "scalar")
	assert.Contains(t, rec.Body.String(), "openapi.json")
}

// TestDocsPage_PinnedAndCSP guards the documentation page's supply chain: the
// Scalar bundle used to be pulled unpinned from a public CDN with no integrity
// hash and no CSP, while the SPA next door had a strict policy.
func TestDocsPage_PinnedAndCSP(t *testing.T) {
	mux := http.NewServeMux()
	RegisterOpenAPI(mux, openapi.BuildSpec())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/docs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// The script is pinned to an exact version and carries an integrity hash, so
	// a substituted bundle fails to execute rather than running on this origin.
	assert.Contains(t, body, "@scalar/api-reference@"+scalarVersion+"/dist/browser/standalone.min.js")
	assert.Contains(t, body, `integrity="`+scalarIntegrity+`"`)
	assert.Contains(t, body, `crossorigin="anonymous"`)
	assert.NotContains(t, body, `src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"`,
		"the unpinned CDN URL must not come back")

	csp := w.Header().Get("Content-Security-Policy")
	require.NotEmpty(t, csp)
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "script-src 'self' https://cdn.jsdelivr.net")
	// The bundle tries to reach api.scalar.com for its "Ask AI" registry; the
	// spec of an internal deployment must not be sent to a third party.
	assert.Contains(t, csp, "connect-src 'self'")
	assert.NotContains(t, csp, "unsafe-eval")

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}
