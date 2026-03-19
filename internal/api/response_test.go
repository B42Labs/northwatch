package api

import (
	"encoding/json"
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

type testModel struct {
	UUID        string            `ovsdb:"_uuid"`
	Name        string            `ovsdb:"name"`
	ExternalIDs map[string]string `ovsdb:"external_ids"`
	Ports       []string          `ovsdb:"ports"`
	Ignored     string
}

func TestModelToMap(t *testing.T) {
	m := testModel{
		UUID:        "abc-123",
		Name:        "test-switch",
		ExternalIDs: map[string]string{"k": "v"},
		Ports:       []string{"p1", "p2"},
		Ignored:     "should not appear",
	}

	result := ModelToMap(m)
	assert.Equal(t, "abc-123", result["_uuid"])
	assert.Equal(t, "test-switch", result["name"])
	assert.Equal(t, map[string]string{"k": "v"}, result["external_ids"])
	assert.Equal(t, []string{"p1", "p2"}, result["ports"])
	_, exists := result["Ignored"]
	assert.False(t, exists)
}

func TestModelToMap_Pointer(t *testing.T) {
	m := &testModel{UUID: "abc-123", Name: "test"}
	result := ModelToMap(m)
	assert.Equal(t, "abc-123", result["_uuid"])
}

func TestModelsToMaps(t *testing.T) {
	models := []testModel{
		{UUID: "1", Name: "a"},
		{UUID: "2", Name: "b"},
	}

	result := ModelsToMaps(models)
	require.Len(t, result, 2)
	assert.Equal(t, "1", result[0]["_uuid"])
	assert.Equal(t, "b", result[1]["name"])
}
