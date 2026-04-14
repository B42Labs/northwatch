package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/events"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocket_FullLifecycle(t *testing.T) {
	hub := events.NewHub()
	mux := http.NewServeMux()
	RegisterWS(mux, hub, nil)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect WebSocket client
	wsURL := "ws" + srv.URL[len("http"):] + "/api/v1/ws"
	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	// Subscribe to nb events
	err = wsjson.Write(ctx, conn, events.SubscribeMessage{
		Action:   "subscribe",
		Database: "nb",
		Tables:   []string{"*"},
	})
	require.NoError(t, err)

	// Give the server a moment to process the subscribe message
	time.Sleep(50 * time.Millisecond)

	// Publish an event
	hub.Publish(events.NewEvent(events.EventInsert, "nb", "Logical_Switch", "test-uuid",
		map[string]any{"name": "ls1"}, nil))

	// Read the event
	var received events.Event
	err = wsjson.Read(ctx, conn, &received)
	require.NoError(t, err)

	assert.Equal(t, events.EventInsert, received.Type)
	assert.Equal(t, "nb", received.Database)
	assert.Equal(t, "Logical_Switch", received.Table)
	assert.Equal(t, "test-uuid", received.UUID)

	// SB events should not arrive
	hub.Publish(events.NewEvent(events.EventInsert, "sb", "Chassis", "sb-uuid", nil, nil))

	// Send a ping
	err = wsjson.Write(ctx, conn, events.SubscribeMessage{Action: "ping"})
	require.NoError(t, err)

	// Read the pong
	var pong map[string]string
	err = wsjson.Read(ctx, conn, &pong)
	require.NoError(t, err)
	assert.Equal(t, "pong", pong["action"])

	// Unsubscribe
	err = wsjson.Write(ctx, conn, events.SubscribeMessage{
		Action:   "unsubscribe",
		Database: "nb",
		Tables:   []string{"*"},
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// After unsubscribe, events should not arrive
	hub.Publish(events.NewEvent(events.EventInsert, "nb", "Logical_Switch", "uuid-2", nil, nil))

	// Send another ping to verify connection is still alive
	err = wsjson.Write(ctx, conn, events.SubscribeMessage{Action: "ping"})
	require.NoError(t, err)

	var pong2 map[string]string
	err = wsjson.Read(ctx, conn, &pong2)
	require.NoError(t, err)
	assert.Equal(t, "pong", pong2["action"])
}

func TestWebSocket_UpdateEvent(t *testing.T) {
	hub := events.NewHub()
	mux := http.NewServeMux()
	RegisterWS(mux, hub, nil)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, "ws"+srv.URL[len("http"):]+"/api/v1/ws", nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	err = wsjson.Write(ctx, conn, events.SubscribeMessage{
		Action:   "subscribe",
		Database: "*",
		Tables:   []string{"*"},
	})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	hub.Publish(events.NewEvent(events.EventUpdate, "nb", "Logical_Switch", "uuid-1",
		map[string]any{"name": "new"}, map[string]any{"name": "old"}))

	var received events.Event
	err = wsjson.Read(ctx, conn, &received)
	require.NoError(t, err)

	assert.Equal(t, events.EventUpdate, received.Type)
	assert.Equal(t, "new", received.Row["name"])
	assert.Equal(t, "old", received.OldRow["name"])
}
