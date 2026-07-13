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

// pingBarrier sends a ping and reads the pong. Control messages are processed
// in order per connection, so a returned pong proves every message written
// before the ping — subscribe/unsubscribe — has already been applied. It
// replaces sleeping to "give the server a moment", making the tests
// deterministic, and doubles as a check that no unexpected event was queued
// ahead of the pong.
func pingBarrier(ctx context.Context, t *testing.T, conn *websocket.Conn) {
	t.Helper()
	require.NoError(t, wsjson.Write(ctx, conn, events.SubscribeMessage{Action: "ping"}))
	var pong map[string]string
	require.NoError(t, wsjson.Read(ctx, conn, &pong))
	require.Equal(t, "pong", pong["action"])
}

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

	// Barrier: the subscribe is applied by the time the pong returns.
	pingBarrier(ctx, t, conn)

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

	// SB events should not arrive; the next pong (not an event) proves it.
	hub.Publish(events.NewEvent(events.EventInsert, "sb", "Chassis", "sb-uuid", nil, nil))
	pingBarrier(ctx, t, conn)

	// Unsubscribe
	err = wsjson.Write(ctx, conn, events.SubscribeMessage{
		Action:   "unsubscribe",
		Database: "nb",
		Tables:   []string{"*"},
	})
	require.NoError(t, err)

	// Barrier: the unsubscribe is applied by the time the pong returns, so the
	// next publish cannot be routed to this subscriber.
	pingBarrier(ctx, t, conn)

	// After unsubscribe, events should not arrive; the next pong proves it.
	hub.Publish(events.NewEvent(events.EventInsert, "nb", "Logical_Switch", "uuid-2", nil, nil))
	pingBarrier(ctx, t, conn)
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
	pingBarrier(ctx, t, conn)

	hub.Publish(events.NewEvent(events.EventUpdate, "nb", "Logical_Switch", "uuid-1",
		map[string]any{"name": "new"}, map[string]any{"name": "old"}))

	var received events.Event
	err = wsjson.Read(ctx, conn, &received)
	require.NoError(t, err)

	assert.Equal(t, events.EventUpdate, received.Type)
	assert.Equal(t, "new", received.Row["name"])
	assert.Equal(t, "old", received.OldRow["name"])
}
