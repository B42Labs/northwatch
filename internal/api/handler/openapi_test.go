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
