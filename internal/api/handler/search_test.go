package handler

import (
	"context"
	"encoding/json"
	"fmt"
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
	engine := search.NewEngine(nil)
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

	engine := search.NewEngine([]search.DatabaseTables{{Name: "nb", Tables: nbTables}})
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
	engine := search.NewEngine(nil)
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

// TestSearchHandler_TruncatedFlag proves the cap is visible to clients: a query
// matching more rows than the engine will return is reported as truncated, so
// the UI can say the result set is a sample rather than the whole answer.
func TestSearchHandler_TruncatedFlag(t *testing.T) {
	rows := make([]searchTestRow, 250)
	for i := range rows {
		rows[i] = searchTestRow{UUID: fmt.Sprintf("uuid-%d", i), Name: fmt.Sprintf("switch-%d", i)}
	}
	engine := search.NewEngine([]search.DatabaseTables{{
		Name: "nb",
		Tables: []search.TableDef{{
			Name:     "Logical_Switch",
			ListFunc: func(context.Context) (any, error) { return rows, nil },
		}},
	}})

	mux := http.NewServeMux()
	RegisterSearch(mux, engine)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/search?q=switch-", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Truncated bool            `json:"truncated"`
		Results   []search.Result `json:"results"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assert.True(t, body.Truncated)
	require.Len(t, body.Results, 1)
	assert.Less(t, len(body.Results[0].Matches), len(rows))
}
