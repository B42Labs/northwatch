package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

func TestGetUUID(t *testing.T) {
	t.Run("nb model pointer", func(t *testing.T) {
		ls := &nb.LogicalSwitch{UUID: "abc-123", Name: "test"}
		assert.Equal(t, "abc-123", getUUID(ls))
	})

	t.Run("sb model pointer", func(t *testing.T) {
		ch := &sb.Chassis{UUID: "def-456", Name: "chassis-1"}
		assert.Equal(t, "def-456", getUUID(ch))
	})

	t.Run("model value (non-pointer)", func(t *testing.T) {
		ls := nb.LogicalSwitch{UUID: "abc-789"}
		assert.Equal(t, "abc-789", getUUID(ls))
	})

	t.Run("struct without UUID field", func(t *testing.T) {
		type other struct{ Name string }
		assert.Equal(t, "", getUUID(&other{Name: "no-uuid"}))
	})
}
