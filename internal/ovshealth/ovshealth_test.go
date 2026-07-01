package ovshealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/ovsdb/vs"
)

// iface builds a vs.Interface with the given link_state and statistics. A "" link
// state leaves LinkState nil (absent from the OVSDB row); a nil stats map is left
// as-is so the nil-map edge is exercised.
func iface(linkState string, stats map[string]int) vs.Interface {
	i := vs.Interface{Statistics: stats}
	if linkState != "" {
		ls := linkState
		i.LinkState = &ls
	}
	return i
}

func TestAggregate(t *testing.T) {
	tests := []struct {
		name   string
		caches []ChassisCache
		want   FleetHealth
	}{
		{
			name:   "empty input is all zero",
			caches: nil,
			want:   FleetHealth{Members: []ChassisHealth{}},
		},
		{
			name: "one connected chassis counts down and erroring interfaces",
			caches: []ChassisCache{{
				SystemID:  "chassis-1",
				Connected: true,
				Bridges:   []vs.Bridge{{}, {}},
				Ports:     []vs.Port{{}, {}, {}},
				Interfaces: []vs.Interface{
					iface("down", nil),                             // down only
					iface("up", map[string]int{"rx_errors": 1}),    // erroring only
					iface("down", map[string]int{"rx_dropped": 2}), // down and erroring
					iface("up", map[string]int{"rx_packets": 9}),   // healthy
				},
			}},
			want: FleetHealth{
				Chassis: 1, Connected: 1, Unreachable: 0,
				Bridges: 2, Ports: 3, Interfaces: 4,
				DownInterfaces: 2, ErrorInterfaces: 2,
				Members: []ChassisHealth{{
					SystemID: "chassis-1", Connected: true,
					Bridges: 2, Ports: 3, Interfaces: 4,
					DownInterfaces: 2, ErrorInterfaces: 2,
				}},
			},
		},
		{
			name: "unreachable chassis is excluded from every total",
			caches: []ChassisCache{{
				SystemID:  "down-node",
				Connected: false,
				// Non-empty slices that must NOT be counted: an unreachable
				// chassis contributes nothing and is never counted as healthy.
				Bridges:    []vs.Bridge{{}, {}},
				Ports:      []vs.Port{{}},
				Interfaces: []vs.Interface{iface("down", map[string]int{"tx_errors": 4})},
			}},
			want: FleetHealth{
				Chassis: 1, Connected: 0, Unreachable: 1,
				Members: []ChassisHealth{{SystemID: "down-node", Connected: false}},
			},
		},
		{
			name: "mixed fleet sums only connected chassis and preserves order",
			caches: []ChassisCache{
				{
					SystemID: "a", Connected: true,
					Bridges:    []vs.Bridge{{}},
					Ports:      []vs.Port{{}, {}},
					Interfaces: []vs.Interface{iface("up", nil), iface("down", nil)},
				},
				{
					SystemID: "b", Connected: false,
					Interfaces: []vs.Interface{iface("down", map[string]int{"rx_errors": 5})},
				},
				{
					SystemID: "c", Connected: true,
					Bridges:    []vs.Bridge{{}, {}, {}},
					Ports:      []vs.Port{{}},
					Interfaces: []vs.Interface{iface("up", map[string]int{"tx_dropped": 1})},
				},
			},
			want: FleetHealth{
				Chassis: 3, Connected: 2, Unreachable: 1,
				Bridges: 4, Ports: 3, Interfaces: 3,
				DownInterfaces: 1, ErrorInterfaces: 1,
				Members: []ChassisHealth{
					{SystemID: "a", Connected: true, Bridges: 1, Ports: 2, Interfaces: 2, DownInterfaces: 1, ErrorInterfaces: 0},
					{SystemID: "b", Connected: false},
					{SystemID: "c", Connected: true, Bridges: 3, Ports: 1, Interfaces: 1, DownInterfaces: 0, ErrorInterfaces: 1},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Aggregate(tc.caches)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestInterfaceDown(t *testing.T) {
	tests := []struct {
		name      string
		linkState string
		want      bool
	}{
		{"nil link_state is not down", "", false},
		{"up is not down", "up", false},
		{"down is down", "down", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, interfaceDown(iface(tc.linkState, nil)))
		})
	}
}

func TestInterfaceErroring(t *testing.T) {
	tests := []struct {
		name  string
		stats map[string]int
		want  bool
	}{
		{"nil statistics is not erroring", nil, false},
		{"all-zero counters are not erroring", map[string]int{"rx_errors": 0, "tx_dropped": 0}, false},
		{"non-error counter alone is not erroring", map[string]int{"rx_packets": 1000}, false},
		{"rx_errors marks erroring", map[string]int{"rx_errors": 1}, true},
		{"tx_dropped alone marks erroring", map[string]int{"tx_dropped": 2}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, interfaceErroring(iface("up", tc.stats)))
		})
	}
}

func TestAggregate_MembersNeverNil(t *testing.T) {
	// Even for empty input, Members is an empty (non-nil) slice so it marshals to
	// a JSON [] rather than null.
	got := Aggregate(nil)
	require.NotNil(t, got.Members)
	assert.Empty(t, got.Members)
}
