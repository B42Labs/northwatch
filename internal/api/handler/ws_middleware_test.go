package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/b42labs/northwatch/internal/telemetry"
	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestWebSocket_ThroughMiddleware(t *testing.T) {
	hub := events.NewHub()
	mux := http.NewServeMux()
	RegisterWS(mux, hub, nil)

	registry := prometheus.NewRegistry()
	m := telemetry.NewMiddleware(registry)
	wrapped := m.Wrap(mux)

	srv := httptest.NewServer(wrapped)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + srv.URL[len("http"):] + "/api/v1/ws"
	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err, "websocket should connect through middleware")
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
}

func TestWebSocket_ThroughMiddleware_RealServer(t *testing.T) {
	hub := events.NewHub()
	mux := http.NewServeMux()
	RegisterWS(mux, hub, nil)

	registry := prometheus.NewRegistry()
	m := telemetry.NewMiddleware(registry)
	wrapped := m.Wrap(mux)

	// Use http.Server.Serve() like production code does
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           wrapped,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addr := ln.Addr().String()
	wsURL := fmt.Sprintf("ws://%s/api/v1/ws", addr)
	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err, "websocket should connect through middleware with real http.Server")
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
}
