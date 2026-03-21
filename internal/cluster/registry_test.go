package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)
	assert.Equal(t, 0, reg.Len())
	assert.Nil(t, reg.Default())
	assert.Empty(t, reg.List())
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	c := &Cluster{Name: "prod", Label: "Production"}
	reg.Register("prod", c)

	got, ok := reg.Get("prod")
	require.True(t, ok)
	assert.Equal(t, "prod", got.Name)
	assert.Equal(t, "Production", got.Label)
}

func TestRegistry_GetMissing(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_Default(t *testing.T) {
	reg := NewRegistry()

	c1 := &Cluster{Name: "first", Label: "First"}
	c2 := &Cluster{Name: "second", Label: "Second"}
	reg.Register("first", c1)
	reg.Register("second", c2)

	def := reg.Default()
	require.NotNil(t, def)
	assert.Equal(t, "first", def.Name)
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()

	c1 := &Cluster{Name: "alpha", Label: "Alpha"}
	c2 := &Cluster{Name: "beta", Label: "Beta"}
	c3 := &Cluster{Name: "gamma", Label: "Gamma"}
	reg.Register("alpha", c1)
	reg.Register("beta", c2)
	reg.Register("gamma", c3)

	list := reg.List()
	require.Len(t, list, 3)
	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "beta", list[1].Name)
	assert.Equal(t, "gamma", list[2].Name)
}

func TestRegistry_Len(t *testing.T) {
	reg := NewRegistry()
	assert.Equal(t, 0, reg.Len())

	reg.Register("a", &Cluster{Name: "a"})
	assert.Equal(t, 1, reg.Len())

	reg.Register("b", &Cluster{Name: "b"})
	assert.Equal(t, 2, reg.Len())
}

func TestRegistry_RegisterOverwrite(t *testing.T) {
	reg := NewRegistry()

	c1 := &Cluster{Name: "prod", Label: "v1"}
	c2 := &Cluster{Name: "prod", Label: "v2"}
	reg.Register("prod", c1)
	reg.Register("prod", c2)

	// Should not duplicate the order entry
	assert.Equal(t, 1, reg.Len())

	got, ok := reg.Get("prod")
	require.True(t, ok)
	assert.Equal(t, "v2", got.Label)

	list := reg.List()
	require.Len(t, list, 1)
	assert.Equal(t, "v2", list[0].Label)
}

func TestRegistry_DefaultEmpty(t *testing.T) {
	reg := NewRegistry()
	assert.Nil(t, reg.Default())
}
