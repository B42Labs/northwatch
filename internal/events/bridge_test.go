package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// testModel mimics a minimal OVSDB model struct for testing.
type testModel struct {
	UUID string `ovsdb:"_uuid"`
	Name string `ovsdb:"name"`
}

func TestBridge_OnAdd(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)
	s.AddFilter(Filter{Database: "nb", Tables: []string{"*"}})

	bridge := NewBridge(hub, "nb")
	bridge.OnAdd("Logical_Switch", &testModel{UUID: "uuid-1", Name: "ls1"})

	select {
	case e := <-s.C:
		assert.Equal(t, EventInsert, e.Type)
		assert.Equal(t, "nb", e.Database)
		assert.Equal(t, "Logical_Switch", e.Table)
		assert.Equal(t, "uuid-1", e.UUID)
		assert.Equal(t, "ls1", e.Row["name"])
		assert.Nil(t, e.OldRow)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBridge_OnUpdate(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)
	s.AddFilter(Filter{Database: "sb", Tables: []string{"*"}})

	bridge := NewBridge(hub, "sb")
	bridge.OnUpdate("Chassis",
		&testModel{UUID: "uuid-2", Name: "old-name"},
		&testModel{UUID: "uuid-2", Name: "new-name"},
	)

	select {
	case e := <-s.C:
		assert.Equal(t, EventUpdate, e.Type)
		assert.Equal(t, "sb", e.Database)
		assert.Equal(t, "uuid-2", e.UUID)
		assert.Equal(t, "new-name", e.Row["name"])
		assert.Equal(t, "old-name", e.OldRow["name"])
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBridge_OnDelete(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)
	s.AddFilter(Filter{Database: "nb", Tables: []string{"Logical_Switch"}})

	bridge := NewBridge(hub, "nb")
	bridge.OnDelete("Logical_Switch", &testModel{UUID: "uuid-3", Name: "deleted"})

	select {
	case e := <-s.C:
		assert.Equal(t, EventDelete, e.Type)
		assert.Equal(t, "uuid-3", e.UUID)
		assert.Equal(t, "deleted", e.Row["name"])
		assert.Nil(t, e.OldRow)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
