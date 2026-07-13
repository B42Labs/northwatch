package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testToken = "0123456789abcdef"

// echoActor answers 200 with the actor the middleware derived from the
// credential, so tests can assert both the gate and the actor it attaches.
func echoActor() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"actor": ActorFromContext(r.Context())})
	})
}

func TestAuthMiddleware(t *testing.T) {
	tokens := map[string]string{"ops": testToken}

	tests := []struct {
		name       string
		method     string
		path       string
		authHeader string
		wantStatus int
		wantActor  string
	}{
		{
			name:       "read route needs no token",
			method:     http.MethodGet,
			path:       "/api/v1/nb/logical-switches",
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-api post passes through",
			method:     http.MethodPost,
			path:       "/some-proxy-hook",
			wantStatus: http.StatusOK,
		},
		{
			name:       "mutating route without token is rejected",
			method:     http.MethodPost,
			path:       "/api/v1/snapshots",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong token is rejected",
			method:     http.MethodPost,
			path:       "/api/v1/snapshots",
			authHeader: "Bearer fedcba9876543210",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong scheme is rejected",
			method:     http.MethodPost,
			path:       "/api/v1/snapshots",
			authHeader: "Basic " + testToken,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token authorizes and names the actor",
			method:     http.MethodPost,
			path:       "/api/v1/snapshots",
			authHeader: "Bearer " + testToken,
			wantStatus: http.StatusOK,
			wantActor:  "ops",
		},
		{
			name:       "scheme match is case-insensitive",
			method:     http.MethodDelete,
			path:       "/api/v1/snapshots/1",
			authHeader: "bearer " + testToken,
			wantStatus: http.StatusOK,
			wantActor:  "ops",
		},
		{
			name:       "put is gated too",
			method:     http.MethodPut,
			path:       "/api/v1/alerts/rules/stale-chassis",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			AuthMiddleware(tokens)(echoActor()).ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantStatus == http.StatusUnauthorized {
				assert.Equal(t, `Bearer realm="northwatch"`, rec.Header().Get("WWW-Authenticate"))
				assert.Contains(t, rec.Body.String(), "authentication required")
				return
			}
			assert.Contains(t, rec.Body.String(), `"actor":"`+tc.wantActor+`"`)
		})
	}
}

func TestAuthMiddleware_NoTokensFailsClosed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/write/preview", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()

	AuthMiddleware(nil)(echoActor()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_NoTokensStillServesReads(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nb/logical-switches", nil)
	rec := httptest.NewRecorder()

	AuthMiddleware(nil)(echoActor()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_QueryStringTokenRejected(t *testing.T) {
	// RFC 6750: credentials come from the Authorization header only. A token in
	// the query string leaks into access logs and Referer headers.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots?access_token="+testToken, nil)
	rec := httptest.NewRecorder()

	AuthMiddleware(map[string]string{"ops": testToken})(echoActor()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_ThrottlesRepeatedFailures(t *testing.T) {
	mw := AuthMiddleware(map[string]string{"ops": testToken})(echoActor())
	post := func(authHeader string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/snapshots", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		return rec
	}

	// The failure budget still answers 401 so an operator typo is not masked.
	for i := 0; i < authFailureLimit; i++ {
		require.Equal(t, http.StatusUnauthorized, post("Bearer wrong").Code)
	}

	// The next attempt from the same source is throttled — even with a valid
	// token, because the lockout precedes the comparison.
	rec := post("Bearer " + testToken)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestAuthMiddleware_SuccessClearsFailures(t *testing.T) {
	mw := AuthMiddleware(map[string]string{"ops": testToken})(echoActor())
	post := func(authHeader string) int {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/snapshots", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		return rec.Code
	}

	// Approach the limit without crossing it.
	for i := 0; i < authFailureLimit-1; i++ {
		require.Equal(t, http.StatusUnauthorized, post("Bearer wrong"))
	}
	// A valid token clears the record, so the full failure budget is available
	// again — no early 429 across the next authFailureLimit rejections.
	require.Equal(t, http.StatusOK, post("Bearer "+testToken))
	for i := 0; i < authFailureLimit; i++ {
		require.Equal(t, http.StatusUnauthorized, post("Bearer wrong"))
	}
	require.Equal(t, http.StatusTooManyRequests, post("Bearer wrong"))
}

func TestAuthThrottle_WindowExpiryResets(t *testing.T) {
	clock := time.Unix(0, 0).UTC()
	th := newAuthThrottle()
	th.now = func() time.Time { return clock }

	const ip = "198.51.100.7"
	for i := 0; i < authFailureLimit; i++ {
		th.fail(ip)
	}
	require.True(t, th.blocked(ip))

	// Once the window elapses the lockout lifts without any sleeping.
	clock = clock.Add(authFailureWindow)
	require.False(t, th.blocked(ip))
}

func TestAuthThrottle_BoundsTrackedIPs(t *testing.T) {
	fill := func(th *authThrottle) {
		for i := 0; i < authThrottleMaxIPs; i++ {
			th.fail(fmt.Sprintf("10.%d.%d.1", i/256, i%256))
		}
	}

	t.Run("sweeps stale entries when the table is full", func(t *testing.T) {
		clock := time.Unix(0, 0).UTC()
		th := newAuthThrottle()
		th.now = func() time.Time { return clock }
		fill(th)
		require.Len(t, th.entries, authThrottleMaxIPs)

		clock = clock.Add(authFailureWindow) // every entry is now stale
		th.fail("203.0.113.1")
		require.Len(t, th.entries, 1)
	})

	t.Run("drops a new sample when the table is full of live entries", func(t *testing.T) {
		clock := time.Unix(0, 0).UTC()
		th := newAuthThrottle()
		th.now = func() time.Time { return clock }
		fill(th)

		th.fail("203.0.113.2") // nothing to sweep, so the new source is not tracked
		require.Len(t, th.entries, authThrottleMaxIPs)
		require.NotContains(t, th.entries, "203.0.113.2")
	})
}

func TestClientIP(t *testing.T) {
	// The throttle must key on the IP, not the ephemeral port that changes per
	// connection, or repeated guesses from one host would each look unique.
	assert.Equal(t, "203.0.113.9", clientIP("203.0.113.9:54321"))
	assert.Equal(t, "203.0.113.9", clientIP("203.0.113.9"))
}

func TestActorFromContext_Absent(t *testing.T) {
	assert.Equal(t, "", ActorFromContext(t.Context()))
}
