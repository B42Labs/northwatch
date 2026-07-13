package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverMiddleware_PanicBecomesGeneric500(t *testing.T) {
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom: /etc/northwatch/secret.db is unreadable")
	})

	rec := httptest.NewRecorder()
	RecoverMiddleware(panicking).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nb/acls", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal server error")
	// The panic value may name internal paths; it belongs in the log, not the
	// response body.
	assert.NotContains(t, rec.Body.String(), "secret.db")
}

func TestRecoverMiddleware_PassesThroughSuccess(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	rec := httptest.NewRecorder()
	RecoverMiddleware(ok).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"ok"`)
}

func TestRecoverMiddleware_RepanicsErrAbortHandler(t *testing.T) {
	// ErrAbortHandler is the standard library's deliberate abort signal, not a
	// bug: swallowing it would turn an intentionally-dropped response into a 500.
	aborting := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(http.ErrAbortHandler)
	})

	assert.PanicsWithError(t, http.ErrAbortHandler.Error(), func() {
		RecoverMiddleware(aborting).ServeHTTP(
			httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil))
	})
}
