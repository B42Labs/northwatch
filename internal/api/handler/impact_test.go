package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/impact"
	"github.com/b42labs/northwatch/internal/testutil"
)

func setupImpactMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()

	nbClient := testutil.SetupNBTestClient(t)
	resolver := impact.NewResolver(nbClient, nil)

	switchUUID := testutil.InsertLogicalSwitch(t, nbClient, "impact-test-sw")

	mux := http.NewServeMux()
	RegisterImpact(mux, resolver)
	return mux, switchUUID
}

func TestHandleImpact_ValidLookup(t *testing.T) {
	mux, switchUUID := setupImpactMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/impact/nb/Logical_Switch/"+switchUUID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var result impact.ImpactResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "impact-test-sw", result.Root.Name)
	assert.Equal(t, "Logical_Switch", result.Root.Table)
	assert.Equal(t, switchUUID, result.Root.UUID)
}

func TestHandleImpact_UnknownDB(t *testing.T) {
	mux, switchUUID := setupImpactMux(t)

	// "sb" is not supported by the resolver.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/impact/sb/Chassis/"+switchUUID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleImpact_UnknownTable(t *testing.T) {
	mux, switchUUID := setupImpactMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/impact/nb/Bogus_Table/"+switchUUID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleImpact_MissingEntity(t *testing.T) {
	mux, _ := setupImpactMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/impact/nb/Logical_Switch/00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

