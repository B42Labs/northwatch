package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/b42labs/northwatch/internal/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type searchTestRow struct {
	UUID string `ovsdb:"_uuid"`
	Name string `ovsdb:"name"`
}

func TestSearchHandler_MissingQuery(t *testing.T) {
	engine := search.NewEngine(nil, nil)
	mux := http.NewServeMux()
	RegisterSearch(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchHandler_WithResults(t *testing.T) {
	nbTables := []search.TableDef{{
		Name: "Logical_Switch",
		ListFunc: func(ctx context.Context) (any, error) {
			return []searchTestRow{
				{UUID: "1", Name: "my-network"},
				{UUID: "2", Name: "other"},
			}, nil
		},
	}}

	engine := search.NewEngine(nbTables, nil)
	mux := http.NewServeMux()
	RegisterSearch(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/search?q=my-network", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "my-network", body["query"])
	assert.Equal(t, "text", body["query_type"])

	results, ok := body["results"].([]any)
	require.True(t, ok)
	assert.Len(t, results, 1)
}

func TestSearchHandler_IPQuery(t *testing.T) {
	engine := search.NewEngine(nil, nil)
	mux := http.NewServeMux()
	RegisterSearch(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/search?q=10.0.0.1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ipv4", body["query_type"])
}
