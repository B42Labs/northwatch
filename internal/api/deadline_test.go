package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deadlineRecorder is an httptest.ResponseRecorder that also records the read
// and write deadlines set on it, which is how http.ResponseController reaches
// the underlying connection in production.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	readDeadline  time.Time
	writeDeadline time.Time
	readSet       bool
	writeSet      bool
}

func newDeadlineRecorder() *deadlineRecorder {
	return &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (d *deadlineRecorder) SetReadDeadline(t time.Time) error {
	d.readDeadline, d.readSet = t, true
	return nil
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.writeDeadline, d.writeSet = t, true
	return nil
}

func TestDeadlineMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantWriteZero bool // WebSocket paths clear the deadline instead of setting one
		wantReadSet   bool
	}{
		{name: "regular api request", path: "/api/v1/nb/acls"},
		{name: "websocket", path: "/api/v1/ws", wantWriteZero: true, wantReadSet: true},
		{name: "cluster-proxied websocket", path: "/api/v1/clusters/prod/ws", wantWriteZero: true, wantReadSet: true},
		{name: "path merely containing ws", path: "/api/v1/clusters/prod/nb/acls"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := newDeadlineRecorder()
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			DeadlineMiddleware(60*time.Second)(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			require.Equal(t, http.StatusOK, rec.Code)
			require.True(t, rec.writeSet, "write deadline must always be touched")
			assert.Equal(t, tc.wantReadSet, rec.readSet)

			if tc.wantWriteZero {
				// A long-lived stream must not inherit a write deadline, and the
				// server's ReadTimeout must not tear down an idle connection.
				assert.True(t, rec.writeDeadline.IsZero())
				assert.True(t, rec.readDeadline.IsZero())
				return
			}
			assert.False(t, rec.writeDeadline.IsZero())
			assert.WithinDuration(t, time.Now().Add(60*time.Second), rec.writeDeadline, 5*time.Second)
		})
	}
}

func TestDeadlineMiddleware_WriterWithoutDeadlineSupport(t *testing.T) {
	// A ResponseWriter that cannot carry deadlines (an httptest recorder) must
	// not fail the request — the controller errors are advisory.
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	DeadlineMiddleware(60*time.Second)(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nb/acls", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestNewServer_SetsTimeouts(t *testing.T) {
	srv := NewServer("127.0.0.1:0", nil)

	assert.Equal(t, 10*time.Second, srv.httpServer.ReadHeaderTimeout)
	assert.Equal(t, 120*time.Second, srv.httpServer.ReadTimeout)
	assert.Equal(t, 120*time.Second, srv.httpServer.IdleTimeout)
	// A blanket WriteTimeout would tear down the WebSocket stream; the per-request
	// deadline middleware handles it instead.
	assert.Zero(t, srv.httpServer.WriteTimeout)
}
