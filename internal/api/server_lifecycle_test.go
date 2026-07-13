package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer_WiringAndAccessors(t *testing.T) {
	// A wrapper records that it saw the request, proving NewServer composes the
	// wrappers around the mux. dbs is nil, so the stale predicate stays disabled.
	var wrapped bool
	wrapper := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapped = true
			next.ServeHTTP(w, r)
		})
	}

	s := NewServer("127.0.0.1:0", nil, wrapper)
	require.NotNil(t, s.Mux())
	assert.Nil(t, s.Databases(), "Databases returns the (nil) bundle it was built with")

	s.Mux().HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	w := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil))

	assert.True(t, wrapped, "the wrapper must run around the mux")
	assert.Equal(t, http.StatusTeapot, w.Code)
}

func TestServer_ListenAndServeThenShutdown(t *testing.T) {
	s := NewServer("127.0.0.1:0", nil)

	errCh := make(chan error, 1)
	go func() { errCh <- s.ListenAndServe(context.Background()) }()

	// Shutdown is idempotent; retry it until ListenAndServe returns. This avoids
	// racing the bind and needs no sleep.
	var serveErr error
	require.Eventually(t, func() bool {
		_ = s.Shutdown(context.Background())
		select {
		case serveErr = <-errCh:
			return true
		default:
			return false
		}
	}, 5*time.Second, 20*time.Millisecond)

	assert.ErrorIs(t, serveErr, http.ErrServerClosed)
}

func TestServer_ListenAndServeBindError(t *testing.T) {
	// A port outside the valid range cannot be bound, so ListenAndServe returns
	// the wrapped listen error rather than blocking in Serve.
	s := NewServer("127.0.0.1:99999999", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := s.ListenAndServe(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listen:")
}

func TestWriteJSONList_NilCoercedToEmptyArray(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSONList[string](w, http.StatusOK, nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, "[]", w.Body.String())
}

func TestWriteJSONList_ItemsRendered(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSONList(w, http.StatusOK, []string{"a", "b"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `["a","b"]`, w.Body.String())
}

func TestNonNil(t *testing.T) {
	assert.Equal(t, []int{}, NonNil[int](nil), "nil slice becomes an empty, non-nil slice")

	in := []int{1, 2}
	assert.Equal(t, in, NonNil(in), "a non-nil slice is returned unchanged")
}
