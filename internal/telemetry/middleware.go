package telemetry

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Middleware records HTTP request metrics using Prometheus counters and histograms.
type Middleware struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
}

// NewMiddleware creates HTTP metrics and registers them with the given registry.
func NewMiddleware(registry *prometheus.Registry) *Middleware {
	m := &Middleware{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "northwatch_http_requests_total",
				Help: "Total number of HTTP requests.",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "northwatch_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
	}
	registry.MustRegister(m.requestsTotal, m.requestDuration)
	return m
}

// Wrap returns an http.Handler that records request metrics around next.
//
// Requests are labeled with the route pattern the mux matched, not their raw
// path. Labeling by path — even with UUIDs normalized — let anyone mint a new
// time series per request just by walking made-up URLs, so a 404 scan grew the
// label set without bound.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		// ServeMux records the matched pattern on the request it dispatches, so
		// this is only readable after the handler has run.
		path := routeLabel(r.Pattern)
		status := strconv.Itoa(rw.status)
		duration := time.Since(start).Seconds()

		m.requestsTotal.WithLabelValues(r.Method, path, status).Inc()
		m.requestDuration.WithLabelValues(r.Method, path, status).Observe(duration)
	})
}

// unmatchedLabel is the single label every unrouted request collapses onto, so
// the cardinality of the path label is bounded by the number of routes.
const unmatchedLabel = "unmatched"

// routeLabel turns a ServeMux pattern ("GET /api/v1/nb/acls/{uuid}") into a path
// label, dropping the leading method. An empty pattern means no route matched.
func routeLabel(pattern string) string {
	if pattern == "" {
		return unmatchedLabel
	}
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return path
	}
	return pattern
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(b)
}

// Flush implements http.Flusher, delegating to the underlying writer if supported.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker, delegating to the underlying writer if supported.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("upstream ResponseWriter does not implement http.Hijacker")
}
