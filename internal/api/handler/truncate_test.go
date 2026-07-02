package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateLabel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"shorter than max is unchanged", "abc", 8, "abc"},
		{"exactly max is unchanged", "12345678", 8, "12345678"},
		{"longer than max is cut", "1234567890", 8, "12345678"},
		{"short uuid does not panic", "abc", 8, "abc"},
		{"multi-byte truncates on rune boundary", "αβγδεζηθικ", 8, "αβγδεζηθ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, truncateLabel(tc.in, tc.max))
		})
	}
}
