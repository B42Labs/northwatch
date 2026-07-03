package events

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_SubscribeUnsubscribe(t *testing.T) {
	hub := NewHub()
	assert.False(t, hub.HasSubscriberFor("nb", "Logical_Switch"))

	s := hub.Subscribe()
	s.AddFilter(Filter{Database: "*", Tables: []string{"*"}})
	assert.True(t, hub.HasSubscriberFor("nb", "Logical_Switch"))

	hub.Unsubscribe(s)
	assert.False(t, hub.HasSubscriberFor("nb", "Logical_Switch"))
	// The subscriber channel is closed on unsubscribe.
	_, ok := <-s.C
	assert.False(t, ok, "channel should be closed after unsubscribe")

	// Double unsubscribe is safe (no panic) and leaves no subscriber.
	hub.Unsubscribe(s)
	assert.False(t, hub.HasSubscriberFor("nb", "Logical_Switch"))
}

func TestHub_HasSubscriberFor(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)
	s.AddFilter(Filter{Database: "nb", Tables: []string{"Logical_Switch", "ACL"}})

	assert.True(t, hub.HasSubscriberFor("nb", "Logical_Switch"))
	assert.True(t, hub.HasSubscriberFor("nb", "ACL"))
	assert.False(t, hub.HasSubscriberFor("nb", "Port_Group"), "table not in filter")
	assert.False(t, hub.HasSubscriberFor("sb", "Logical_Switch"), "database mismatch")

	// A subscriber with no filters matches nothing.
	empty := hub.Subscribe()
	defer hub.Unsubscribe(empty)
	assert.False(t, hub.HasSubscriberFor("nb", "NAT"))
}

func TestSubscriber_RecordDropRateLimited(t *testing.T) {
	s := &Subscriber{}
	t0 := time.Unix(1_700_000_000, 0)

	// First drop logs immediately (starting the window).
	n, ok := s.recordDrop(t0)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), n)

	// Further drops within the interval are suppressed but counted.
	_, ok = s.recordDrop(t0.Add(1 * time.Second))
	assert.False(t, ok)
	_, ok = s.recordDrop(t0.Add(2 * time.Second))
	assert.False(t, ok)

	// Past the interval, one summary reports the accumulated count.
	n, ok = s.recordDrop(t0.Add(dropLogInterval + time.Second))
	assert.True(t, ok)
	assert.Equal(t, uint64(3), n, "the two suppressed drops plus this one")
}

func TestHub_PublishToMatchingSubscriber(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	s.AddFilter(Filter{Database: "nb", Tables: []string{"*"}})

	hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid-1", map[string]any{"name": "ls1"}, nil))

	select {
	case e := <-s.C:
		assert.Equal(t, EventInsert, e.Type)
		assert.Equal(t, "nb", e.Database)
		assert.Equal(t, "Logical_Switch", e.Table)
		assert.Equal(t, "uuid-1", e.UUID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestHub_NoEventsWithoutSubscription(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	// No filters added
	hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid-1", nil, nil))

	select {
	case <-s.C:
		t.Fatal("should not receive events without subscription")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestHub_FilterByDatabase(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	s.AddFilter(Filter{Database: "nb", Tables: []string{"*"}})

	hub.Publish(NewEvent(EventInsert, "sb", "Chassis", "uuid-1", nil, nil))

	select {
	case <-s.C:
		t.Fatal("should not receive sb events with nb filter")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestHub_FilterByTable(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	s.AddFilter(Filter{Database: "nb", Tables: []string{"Logical_Switch"}})

	hub.Publish(NewEvent(EventInsert, "nb", "Logical_Router", "uuid-1", nil, nil))

	select {
	case <-s.C:
		t.Fatal("should not receive Logical_Router with Logical_Switch filter")
	case <-time.After(50 * time.Millisecond):
		// expected
	}

	hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid-2", nil, nil))

	select {
	case e := <-s.C:
		assert.Equal(t, "Logical_Switch", e.Table)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestHub_WildcardDatabase(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	s.AddFilter(Filter{Database: "*", Tables: []string{"*"}})

	hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid-1", nil, nil))
	hub.Publish(NewEvent(EventDelete, "sb", "Chassis", "uuid-2", nil, nil))

	e1 := <-s.C
	assert.Equal(t, "nb", e1.Database)
	e2 := <-s.C
	assert.Equal(t, "sb", e2.Database)
}

func TestHub_RemoveFilter(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	s.AddFilter(Filter{Database: "nb", Tables: []string{"*"}})
	s.RemoveFilter("nb", []string{"*"})

	hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid-1", nil, nil))

	select {
	case <-s.C:
		t.Fatal("should not receive events after filter removed")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestHub_SlowSubscriberDropsEvents(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	s.AddFilter(Filter{Database: "*", Tables: []string{"*"}})

	// Fill the buffer
	for i := 0; i < subscriberBufferSize+10; i++ {
		hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid", nil, nil))
	}

	// Should have exactly subscriberBufferSize events
	count := 0
	for {
		select {
		case <-s.C:
			count++
		default:
			goto done
		}
	}
done:
	assert.Equal(t, subscriberBufferSize, count)
}

func TestHub_ConcurrentPublish(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe()
	defer hub.Unsubscribe(s)

	s.AddFilter(Filter{Database: "*", Tables: []string{"*"}})

	var wg sync.WaitGroup
	n := 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid", nil, nil))
		}()
	}
	wg.Wait()

	count := 0
	for {
		select {
		case <-s.C:
			count++
		default:
			goto done2
		}
	}
done2:
	assert.Equal(t, n, count)
}

func TestHub_MultipleSubscribers(t *testing.T) {
	hub := NewHub()
	s1 := hub.Subscribe()
	s2 := hub.Subscribe()
	defer hub.Unsubscribe(s1)
	defer hub.Unsubscribe(s2)

	s1.AddFilter(Filter{Database: "nb", Tables: []string{"*"}})
	s2.AddFilter(Filter{Database: "sb", Tables: []string{"*"}})

	hub.Publish(NewEvent(EventInsert, "nb", "Logical_Switch", "uuid-1", nil, nil))
	hub.Publish(NewEvent(EventInsert, "sb", "Chassis", "uuid-2", nil, nil))

	e1 := <-s1.C
	assert.Equal(t, "nb", e1.Database)

	e2 := <-s2.C
	assert.Equal(t, "sb", e2.Database)

	// s1 should not get the sb event
	select {
	case <-s1.C:
		t.Fatal("s1 should not get sb event")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestFilter_Matches(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		event  Event
		want   bool
	}{
		{
			name:   "wildcard matches all",
			filter: Filter{Database: "*", Tables: []string{"*"}},
			event:  Event{Database: "nb", Table: "Logical_Switch"},
			want:   true,
		},
		{
			name:   "specific db matches",
			filter: Filter{Database: "nb", Tables: []string{"*"}},
			event:  Event{Database: "nb", Table: "Logical_Switch"},
			want:   true,
		},
		{
			name:   "wrong db does not match",
			filter: Filter{Database: "nb", Tables: []string{"*"}},
			event:  Event{Database: "sb", Table: "Chassis"},
			want:   false,
		},
		{
			name:   "specific table matches",
			filter: Filter{Database: "nb", Tables: []string{"Logical_Switch"}},
			event:  Event{Database: "nb", Table: "Logical_Switch"},
			want:   true,
		},
		{
			name:   "wrong table does not match",
			filter: Filter{Database: "nb", Tables: []string{"Logical_Switch"}},
			event:  Event{Database: "nb", Table: "Logical_Router"},
			want:   false,
		},
		{
			name:   "empty tables does not match",
			filter: Filter{Database: "*", Tables: []string{}},
			event:  Event{Database: "nb", Table: "Logical_Switch"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Matches(tt.event)
			require.Equal(t, tt.want, got)
		})
	}
}
