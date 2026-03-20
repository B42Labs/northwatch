package enrich

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	portInfo    *Info
	networkInfo *Info
	routerInfo  *Info
	natInfo     *Info
	err         error
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) EnrichPort(_ context.Context, _ map[string]string) (*Info, error) {
	return m.portInfo, m.err
}
func (m *mockProvider) EnrichNetwork(_ context.Context, _ map[string]string) (*Info, error) {
	return m.networkInfo, m.err
}
func (m *mockProvider) EnrichRouter(_ context.Context, _ map[string]string) (*Info, error) {
	return m.routerInfo, m.err
}
func (m *mockProvider) EnrichNAT(_ context.Context, _ map[string]string) (*Info, error) {
	return m.natInfo, m.err
}

func TestNewEnricher(t *testing.T) {
	t.Run("with provider", func(t *testing.T) {
		e := NewEnricher(&mockProvider{}, 5*time.Minute)
		require.NotNil(t, e)
		assert.NotNil(t, e.cache)
		assert.True(t, e.HasProvider())
	})

	t.Run("nil provider", func(t *testing.T) {
		e := NewEnricher(nil, 5*time.Minute)
		require.NotNil(t, e)
		assert.Nil(t, e.cache)
		assert.False(t, e.HasProvider())
	})
}

func TestEnricher_NilProvider(t *testing.T) {
	e := NewEnricher(nil, time.Minute)
	ctx := context.Background()
	ids := map[string]string{"k": "v"}

	assert.Nil(t, e.EnrichPort(ctx, "id1", ids))
	assert.Nil(t, e.EnrichNetwork(ctx, "id2", ids))
	assert.Nil(t, e.EnrichRouter(ctx, "id3", ids))
	assert.Nil(t, e.EnrichNAT(ctx, "id4", ids))
}

func TestEnricher_EnrichPort(t *testing.T) {
	info := &Info{DisplayName: "port-1"}
	e := NewEnricher(&mockProvider{portInfo: info}, time.Minute)

	result := e.EnrichPort(context.Background(), "p1", map[string]string{"k": "v"})
	require.NotNil(t, result)
	assert.Equal(t, "port-1", result.DisplayName)

	// Second call should return cached result
	result2 := e.EnrichPort(context.Background(), "p1", nil)
	assert.Equal(t, result, result2)
}

func TestEnricher_EnrichNetwork(t *testing.T) {
	info := &Info{DisplayName: "net-1"}
	e := NewEnricher(&mockProvider{networkInfo: info}, time.Minute)

	result := e.EnrichNetwork(context.Background(), "n1", map[string]string{"k": "v"})
	require.NotNil(t, result)
	assert.Equal(t, "net-1", result.DisplayName)
}

func TestEnricher_EnrichRouter(t *testing.T) {
	info := &Info{DisplayName: "router-1"}
	e := NewEnricher(&mockProvider{routerInfo: info}, time.Minute)

	result := e.EnrichRouter(context.Background(), "r1", map[string]string{"k": "v"})
	require.NotNil(t, result)
	assert.Equal(t, "router-1", result.DisplayName)
}

func TestEnricher_EnrichNAT(t *testing.T) {
	info := &Info{DisplayName: "nat-1"}
	e := NewEnricher(&mockProvider{natInfo: info}, time.Minute)

	result := e.EnrichNAT(context.Background(), "nat1", map[string]string{"k": "v"})
	require.NotNil(t, result)
	assert.Equal(t, "nat-1", result.DisplayName)
}

func TestEnricher_ProviderError(t *testing.T) {
	e := NewEnricher(&mockProvider{err: fmt.Errorf("fail")}, time.Minute)

	result := e.EnrichPort(context.Background(), "p1", map[string]string{"k": "v"})
	assert.Nil(t, result)
}

func TestEnricher_ProviderReturnsNil(t *testing.T) {
	e := NewEnricher(&mockProvider{portInfo: nil}, time.Minute)

	result := e.EnrichPort(context.Background(), "p1", map[string]string{"k": "v"})
	assert.Nil(t, result)
}
