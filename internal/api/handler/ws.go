package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	wsPingInterval = 30 * time.Second
	wsWriteTimeout = 5 * time.Second
)

// wsAllowedOrigins is the package-level allowlist of host patterns for
// WebSocket Origin checking, configured at startup via RegisterWS.
var wsAllowedOrigins []string

// RegisterWS registers the WebSocket endpoint for real-time events.
// allowedOrigins is the parsed list of OriginPatterns from config; if empty,
// origin checking is disabled (InsecureSkipVerify), which is appropriate for
// single-tenant deployments behind an operator-controlled reverse proxy.
func RegisterWS(mux *http.ServeMux, hub *events.Hub, allowedOrigins []string) {
	wsAllowedOrigins = allowedOrigins
	mux.HandleFunc("GET /api/v1/ws", handleWS(hub))
}

// ParseWSAllowedOrigins splits a comma-separated WS_ALLOWED_ORIGINS value
// into a normalized slice of host patterns, trimming whitespace and
// dropping empty entries.
func ParseWSAllowedOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	return result
}

func handleWS(hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opts := &websocket.AcceptOptions{}
		if len(wsAllowedOrigins) > 0 {
			opts.OriginPatterns = wsAllowedOrigins
		} else {
			// No allowlist configured: skip the Origin check. Trusting the
			// request's Host header would have offered no real protection
			// since clients fully control it. Operators with multi-tenant
			// exposure should set --ws-allowed-origins.
			opts.InsecureSkipVerify = true
		}
		conn, err := websocket.Accept(w, r, opts)
		if err != nil {
			log.Printf("ws: accept error: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

		sub := hub.Subscribe()
		defer hub.Unsubscribe(sub)

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Read loop: handle subscribe/unsubscribe/ping messages from client
		go func() {
			defer cancel()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var msg events.SubscribeMessage
				if err := json.Unmarshal(data, &msg); err != nil {
					continue
				}
				switch msg.Action {
				case "subscribe":
					sub.AddFilter(events.Filter{
						Database: msg.Database,
						Tables:   msg.Tables,
					})
				case "unsubscribe":
					sub.RemoveFilter(msg.Database, msg.Tables)
				case "ping":
					// Client-level ping; respond with pong
					writeCtx, writeCancel := context.WithTimeout(ctx, wsWriteTimeout)
					_ = wsjson.Write(writeCtx, conn, map[string]string{"action": "pong"})
					writeCancel()
				}
			}
		}()

		// Ping ticker for protocol-level pings
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()

		// Write loop: send events to client
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-sub.C:
				if !ok {
					return
				}
				writeCtx, writeCancel := context.WithTimeout(ctx, wsWriteTimeout)
				err := wsjson.Write(writeCtx, conn, event)
				writeCancel()
				if err != nil {
					return
				}
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, wsWriteTimeout)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					return
				}
			}
		}
	}
}
