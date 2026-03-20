package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	wsPingInterval = 30 * time.Second
	wsWriteTimeout = 5 * time.Second
)

// RegisterWS registers the WebSocket endpoint for real-time events.
func RegisterWS(mux *http.ServeMux, hub *events.Hub) {
	mux.HandleFunc("GET /api/v1/ws", handleWS(hub))
}

func handleWS(hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{r.Host},
		})
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
