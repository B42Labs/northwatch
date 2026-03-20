package enrich

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetSet(t *testing.T) {
	c := NewCache(5 * time.Minute)

	info := &Info{DisplayName: "test-port"}
	c.Set("port:abc", info)

	got, ok := c.Get("port:abc")
	require.True(t, ok)
	assert.Equal(t, "test-port", got.DisplayName)
}

func TestCache_Miss(t *testing.T) {
	c := NewCache(5 * time.Minute)

	_, ok := c.Get("nonexistent")
	assert.False(t, ok)
}

func TestCache_Expiration(t *testing.T) {
	c := NewCache(10 * time.Millisecond)

	c.Set("key", &Info{DisplayName: "expiring"})

	_, ok := c.Get("key")
	require.True(t, ok)

	time.Sleep(20 * time.Millisecond)
	_, ok = c.Get("key")
	assert.False(t, ok)
}

func TestCache_Overwrite(t *testing.T) {
	c := NewCache(5 * time.Minute)

	c.Set("key", &Info{DisplayName: "first"})
	c.Set("key", &Info{DisplayName: "second"})

	got, ok := c.Get("key")
	require.True(t, ok)
	assert.Equal(t, "second", got.DisplayName)
}

func TestCache_Concurrent(t *testing.T) {
	c := NewCache(5 * time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.Set("key", &Info{DisplayName: "value"})
		}()
		go func() {
			defer wg.Done()
			c.Get("key")
		}()
	}

	wg.Wait()
}
