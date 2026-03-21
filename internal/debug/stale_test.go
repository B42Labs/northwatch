package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStaleEntriesResult_Init(t *testing.T) {
	r := &StaleEntriesResult{Entries: []StaleEntry{}}
	assert.Equal(t, 0, r.Total)
	assert.Empty(t, r.Entries)
}

func TestStaleEntry_Types(t *testing.T) {
	types := []string{"mac_binding", "fdb", "port_binding"}
	for _, typ := range types {
		e := StaleEntry{Type: typ}
		assert.Equal(t, typ, e.Type)
	}
}
