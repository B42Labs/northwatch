package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// actorContextKey keys the authenticated token name on a request context.
type actorContextKey struct{}

// ActorFromContext returns the name of the API token that authenticated the
// request, or "" when the request was not authenticated (a read route, or a
// deployment running with --insecure-no-auth). Handlers that record an audit
// trail derive the actor from this rather than from the request body, so it
// cannot be spoofed by the client.
func ActorFromContext(ctx context.Context) string {
	name, _ := ctx.Value(actorContextKey{}).(string)
	return name
}

// AuthMiddleware gates every mutating /api/ request behind one of the configured
// bearer tokens. Read routes (GET/HEAD/OPTIONS) and non-API paths (the SPA, its
// assets, /metrics, /healthz) pass through untouched: this is an authorization
// gate on state changes, not a site-wide login. Protecting the read surface
// remains the job of a reverse proxy, as documented in the deployment guide.
//
// Tokens are accepted only from the Authorization header (RFC 6750) — never from
// a query string, where they would leak into logs and Referer headers — and are
// compared as SHA-256 digests in constant time so a rejection leaks neither the
// length nor a prefix of the configured secret. On success the matching token's
// name is attached to the request context (see ActorFromContext).
//
// An empty token map fails closed: every mutating request is rejected. Callers
// that deliberately run without authentication must not install this middleware
// at all.
//
// Repeated failures from one source IP are throttled (see authThrottle) so the
// constant-time comparison is backed by a lockout that bounds online guessing;
// on a network-exposed listener the primary control remains a reverse proxy.
func AuthMiddleware(tokens map[string]string) func(http.Handler) http.Handler {
	digests := make(map[string][32]byte, len(tokens))
	for name, token := range tokens {
		digests[name] = sha256.Sum256([]byte(token))
	}
	throttle := newAuthThrottle()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !requiresAuth(r) {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r.RemoteAddr)
			if throttle.blocked(ip) {
				// #nosec G706 -- structured slog attributes, not an interpolated message
				slog.Warn("throttled repeated authentication failures",
					"method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
				w.Header().Set("Retry-After", strconv.Itoa(int(authFailureWindow/time.Second)))
				WriteError(w, http.StatusTooManyRequests, "too many authentication failures")
				return
			}

			name, ok := lookupActor(digests, bearerToken(r.Header.Get("Authorization")))
			if !ok {
				throttle.fail(ip)
				slog.Warn("rejected unauthenticated mutating request",
					"method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
				w.Header().Set("WWW-Authenticate", `Bearer realm="northwatch"`)
				WriteError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			throttle.succeed(ip)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorContextKey{}, name)))
		})
	}
}

const (
	// authFailureLimit is how many failed attempts one source IP may make within
	// authFailureWindow before it is locked out for the rest of the window. The
	// budget leaves room for operator typos while capping online token guessing.
	authFailureLimit  = 5
	authFailureWindow = time.Minute
	// authThrottleMaxIPs bounds the tracking map so a spray from many distinct
	// (possibly spoofed) source IPs cannot grow it without limit.
	authThrottleMaxIPs = 4096
)

// throttleEntry counts recent failures from one source IP. The entry is stale
// once now is no longer before resets: the window has elapsed and the count
// starts over.
type throttleEntry struct {
	count  int
	resets time.Time
}

// authThrottle blunts online guessing of bearer tokens by locking out a source
// IP after authFailureLimit failed attempts within authFailureWindow. It is
// defense in depth behind the constant-time comparison in lookupActor; on a
// network-exposed listener the primary control remains a reverse proxy (see
// AuthMiddleware). now is a seam so tests can advance the clock without sleeping.
type authThrottle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
	now     func() time.Time
}

func newAuthThrottle() *authThrottle {
	return &authThrottle{entries: make(map[string]*throttleEntry), now: time.Now}
}

// blocked reports whether ip is currently locked out, dropping a stale entry on
// the way so a quiet IP does not stay tracked past its window.
func (t *authThrottle) blocked(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[ip]
	if e == nil {
		return false
	}
	if !t.now().Before(e.resets) {
		delete(t.entries, ip)
		return false
	}
	return e.count >= authFailureLimit
}

// fail records one failed attempt from ip.
func (t *authThrottle) fail(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	e := t.entries[ip]
	if e == nil || !now.Before(e.resets) {
		if len(t.entries) >= authThrottleMaxIPs && !t.sweep(now) {
			// The table is full of live entries: drop this sample rather than
			// grow unbounded. Memory stays capped and the flooding IPs stay
			// locked; only a brand-new IP momentarily escapes tracking.
			return
		}
		e = &throttleEntry{resets: now.Add(authFailureWindow)}
		t.entries[ip] = e
	}
	e.count++
}

// succeed clears any failure record for ip once it authenticates.
func (t *authThrottle) succeed(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, ip)
}

// sweep drops stale entries and reports whether it freed any room. The caller
// must hold t.mu; it runs only when the table is at capacity, so its O(n) cost
// is amortized across authThrottleMaxIPs additions.
func (t *authThrottle) sweep(now time.Time) bool {
	before := len(t.entries)
	for ip, e := range t.entries {
		if !now.Before(e.resets) {
			delete(t.entries, ip)
		}
	}
	return len(t.entries) < before
}

// clientIP returns the host portion of a RemoteAddr so the throttle keys on the
// source IP rather than its ephemeral port, which changes per connection.
func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// requiresAuth reports whether a request mutates state and therefore needs a
// token. Every mutating Northwatch route is a non-GET method under /api/.
func requiresAuth(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return strings.HasPrefix(r.URL.Path, "/api/")
}

// bearerToken extracts the credential from an Authorization header value,
// returning "" when the scheme is absent or is not Bearer.
func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// lookupActor returns the name of the token matching the presented credential.
// Every configured digest is compared (no early exit) so the work does not
// depend on which token matched.
func lookupActor(digests map[string][32]byte, presented string) (string, bool) {
	if presented == "" {
		return "", false
	}
	got := sha256.Sum256([]byte(presented))

	var match string
	for name, want := range digests {
		if subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
			match = name
		}
	}
	return match, match != ""
}
