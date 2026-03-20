package telemetry

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

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
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		path := normalizePath(r.URL.Path)
		status := strconv.Itoa(rw.status)
		duration := time.Since(start).Seconds()

		m.requestsTotal.WithLabelValues(r.Method, path, status).Inc()
		m.requestDuration.WithLabelValues(r.Method, path, status).Observe(duration)
	})
}

// normalizePath replaces UUIDs with {uuid} to avoid label cardinality explosion.
func normalizePath(path string) string {
	return uuidPattern.ReplaceAllString(path, "{uuid}")
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
