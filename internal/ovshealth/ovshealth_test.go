package ovshealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/ovsdb/vs"
)

// iface builds a vs.Interface with the given uuid, link_state and statistics. A
// "" link state leaves LinkState nil; a nil stats map exercises the nil-map edge.
func iface(uuid, linkState string, stats map[string]int) vs.Interface {
	i := vs.Interface{UUID: uuid, Statistics: stats}
	if linkState != "" {
		ls := linkState
		i.LinkState = &ls
	}
	return i
}

// TestAggregateStructural covers the non-delta accounting: bridge/port/interface
// and down-interface counts, unreachable exclusion, and input ordering. On a
// tracker's first observation nothing is flagged erroring/dropping.
func TestAggregateStructural(t *testing.T) {
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
			name: "first observation flags nothing erroring or dropping",
			caches: []ChassisCache{{
				SystemID:  "chassis-1",
				Connected: true,
				Bridges:   []vs.Bridge{{}, {}},
				Ports:     []vs.Port{{}, {}, {}},
				Interfaces: []vs.Interface{
					iface("if-a", "down", nil),                             // down only
					iface("if-b", "up", map[string]int{"rx_errors": 1}),    // lifetime error, baseline
					iface("if-c", "down", map[string]int{"rx_dropped": 2}), // down + lifetime drop, baseline
					iface("if-d", "up", map[string]int{"rx_packets": 9}),   // healthy
				},
			}},
			want: FleetHealth{
				Chassis: 1, Connected: 1, Unreachable: 0,
				Bridges: 2, Ports: 3, Interfaces: 4,
				DownInterfaces: 2, ErrorInterfaces: 0, DropInterfaces: 0,
				Members: []ChassisHealth{{
					SystemID: "chassis-1", Connected: true,
					Bridges: 2, Ports: 3, Interfaces: 4,
					DownInterfaces: 2,
				}},
			},
		},
		{
			name: "unreachable chassis is excluded from every total",
			caches: []ChassisCache{{
				SystemID:   "down-node",
				Connected:  false,
				Bridges:    []vs.Bridge{{}, {}},
				Ports:      []vs.Port{{}},
				Interfaces: []vs.Interface{iface("x", "down", map[string]int{"tx_errors": 4})},
			}},
			want: FleetHealth{
				Chassis: 1, Connected: 0, Unreachable: 1,
				Members: []ChassisHealth{{SystemID: "down-node", Connected: false}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewTracker().Aggregate(tc.caches)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestTrackerFlagsOnCounterIncrease verifies errors and drops are flagged only
// when the counters advance between aggregations, and reported separately.
func TestTrackerFlagsOnCounterIncrease(t *testing.T) {
	tr := NewTracker()

	cache := func(rxErr, rxDrop int) []ChassisCache {
		return []ChassisCache{{
			SystemID:   "c1",
			Connected:  true,
			Interfaces: []vs.Interface{iface("if-1", "up", map[string]int{"rx_errors": rxErr, "rx_dropped": rxDrop})},
		}}
	}

	// First observation establishes the baseline: even with non-zero counters,
	// nothing is flagged.
	got := tr.Aggregate(cache(5, 3))
	assert.Equal(t, 0, got.ErrorInterfaces)
	assert.Equal(t, 0, got.DropInterfaces)

	// Errors advance, drops unchanged -> only erroring.
	got = tr.Aggregate(cache(6, 3))
	assert.Equal(t, 1, got.ErrorInterfaces)
	assert.Equal(t, 0, got.DropInterfaces)

	// Drops advance, errors unchanged -> only dropping.
	got = tr.Aggregate(cache(6, 4))
	assert.Equal(t, 0, got.ErrorInterfaces)
	assert.Equal(t, 1, got.DropInterfaces)

	// No change -> nothing flagged.
	got = tr.Aggregate(cache(6, 4))
	assert.Equal(t, 0, got.ErrorInterfaces)
	assert.Equal(t, 0, got.DropInterfaces)
}

// TestTrackerCounterReset verifies a cumulative-counter reset (e.g. after a
// device restart) re-baselines instead of flagging a spurious error.
func TestTrackerCounterReset(t *testing.T) {
	tr := NewTracker()
	cache := func(rxErr int) []ChassisCache {
		return []ChassisCache{{
			SystemID:   "c1",
			Connected:  true,
			Interfaces: []vs.Interface{iface("if-1", "up", map[string]int{"rx_errors": rxErr})},
		}}
	}

	tr.Aggregate(cache(100)) // baseline
	got := tr.Aggregate(cache(0))
	assert.Equal(t, 0, got.ErrorInterfaces, "a counter reset must not be flagged as erroring")

	// After the reset, a genuine increase is flagged again.
	got = tr.Aggregate(cache(1))
	assert.Equal(t, 1, got.ErrorInterfaces)
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
			assert.Equal(t, tc.want, interfaceDown(iface("i", tc.linkState, nil)))
		})
	}
}

func TestAggregate_MembersNeverNil(t *testing.T) {
	// Even for empty input, Members is an empty (non-nil) slice so it marshals to
	// a JSON [] rather than null.
	got := NewTracker().Aggregate(nil)
	require.NotNil(t, got.Members)
	assert.Empty(t, got.Members)
}
