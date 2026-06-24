package router

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/testutil"
)

// refNow is a fixed reference time so MAC_Binding timestamps can be set
// deterministically relative to "now".
var refNow = time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

func newAnalyzer(t *testing.T) *Analyzer {
	t.Helper()
	return &Analyzer{
		NB:  testutil.SetupNBTestClient(t),
		SB:  testutil.SetupSBTestClient(t),
		Now: func() time.Time { return refNow },
	}
}

func analyze(t *testing.T, a *Analyzer) *Report {
	t.Helper()
	rep, err := a.Analyze(context.Background())
	require.NoError(t, err)
	return rep
}

// findNextHop returns the single next hop for the given address.
func findNextHop(t *testing.T, rep *Report, nexthop string) NextHop {
	t.Helper()
	for _, h := range rep.NextHops {
		if h.Nexthop == nexthop {
			return h
		}
	}
	t.Fatalf("next hop %q not found in report", nexthop)
	return NextHop{}
}

func TestAnalyze_Empty(t *testing.T) {
	a := newAnalyzer(t)
	rep := analyze(t, a)
	assert.Equal(t, 0, rep.Total)
	assert.Empty(t, rep.NextHops)
}

func TestAnalyze_NoAging(t *testing.T) {
	a := newAnalyzer(t)
	// Router with no mac_binding_age_threshold -> learned MACs never expire.
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"172.18.16.1/24"}, nil,
		[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "172.18.16.1"}})
	testutil.InsertMACBinding(t, a.SB, "EXT", "172.18.16.1", "c2:f9:61:ef:64:e0",
		int(refNow.Add(-time.Hour).UnixMilli()))

	rep := analyze(t, a)
	require.Equal(t, 1, rep.Total)
	assert.Equal(t, 1, rep.Warning)
	assert.Equal(t, 1, rep.NoAging)

	hop := findNextHop(t, rep, "172.18.16.1")
	assert.Equal(t, StatusNoAging, hop.Status)
	assert.Equal(t, SeverityWarning, hop.Overall)
	assert.Equal(t, "c2:f9:61:ef:64:e0", hop.CachedMAC)
	assert.Equal(t, "EXT", hop.LRPName)
	assert.False(t, hop.AgingEnabled)
	assert.NotEmpty(t, hop.MACBindingUUID)
}

func TestAnalyze_OKWithAging(t *testing.T) {
	a := newAnalyzer(t)
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"172.18.16.1/24"},
		map[string]string{"mac_binding_age_threshold": "300"},
		[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "172.18.16.1"}})
	// Learned 60s ago, threshold 300s -> fresh.
	testutil.InsertMACBinding(t, a.SB, "EXT", "172.18.16.1", "c2:f9:61:ef:64:e0",
		int(refNow.Add(-60*time.Second).UnixMilli()))

	rep := analyze(t, a)
	require.Equal(t, 1, rep.Total)
	assert.Equal(t, 1, rep.Healthy)
	hop := findNextHop(t, rep, "172.18.16.1")
	assert.Equal(t, StatusOK, hop.Status)
	assert.True(t, hop.AgingEnabled)
	assert.Equal(t, 300, hop.AgeThreshold)
}

func TestAnalyze_Stale(t *testing.T) {
	a := newAnalyzer(t)
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"172.18.16.1/24"},
		map[string]string{"mac_binding_age_threshold": "60"},
		[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "172.18.16.1"}})
	// Learned an hour ago, threshold 60s -> northd should have aged it.
	testutil.InsertMACBinding(t, a.SB, "EXT", "172.18.16.1", "c2:f9:61:ef:64:e0",
		int(refNow.Add(-time.Hour).UnixMilli()))

	rep := analyze(t, a)
	assert.Equal(t, 1, rep.Stale)
	hop := findNextHop(t, rep, "172.18.16.1")
	assert.Equal(t, StatusStale, hop.Status)
	assert.Equal(t, SeverityWarning, hop.Overall)
	assert.Greater(t, hop.AgeSeconds, int64(60))
}

func TestAnalyze_GlobalAge(t *testing.T) {
	a := newAnalyzer(t)
	// Threshold only set globally on NB_Global; the router inherits it.
	testutil.InsertNBGlobalWithOptions(t, a.NB, map[string]string{"mac_binding_age_threshold": "300"})
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"172.18.16.1/24"}, nil,
		[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "172.18.16.1"}})
	testutil.InsertMACBinding(t, a.SB, "EXT", "172.18.16.1", "c2:f9:61:ef:64:e0",
		int(refNow.Add(-60*time.Second).UnixMilli()))

	rep := analyze(t, a)
	hop := findNextHop(t, rep, "172.18.16.1")
	assert.True(t, hop.AgingEnabled)
	assert.Equal(t, StatusOK, hop.Status)
}

func TestAnalyze_Pinned(t *testing.T) {
	a := newAnalyzer(t)
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"172.18.16.1/24"}, nil,
		[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "172.18.16.1"}})
	testutil.InsertStaticMACBinding(t, a.SB, "EXT", "172.18.16.1", "aa:bb:cc:dd:ee:ff", true)

	rep := analyze(t, a)
	assert.Equal(t, 1, rep.Healthy)
	hop := findNextHop(t, rep, "172.18.16.1")
	assert.Equal(t, StatusPinned, hop.Status)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", hop.StaticMAC)
	assert.True(t, hop.Override)
}

func TestAnalyze_Conflict(t *testing.T) {
	a := newAnalyzer(t)
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"172.18.16.1/24"}, nil,
		[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "172.18.16.1"}})
	testutil.InsertMACBinding(t, a.SB, "EXT", "172.18.16.1", "c2:f9:61:ef:64:e0",
		int(refNow.Add(-time.Hour).UnixMilli()))
	// Static pin disagrees and does NOT override -> ambiguous.
	testutil.InsertStaticMACBinding(t, a.SB, "EXT", "172.18.16.1", "aa:bb:cc:dd:ee:ff", false)

	rep := analyze(t, a)
	assert.Equal(t, 1, rep.Conflict)
	hop := findNextHop(t, rep, "172.18.16.1")
	assert.Equal(t, StatusConflict, hop.Status)
	assert.Equal(t, SeverityWarning, hop.Overall)
}

func TestAnalyze_Unresolved(t *testing.T) {
	a := newAnalyzer(t)
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"172.18.16.1/24"}, nil,
		[]testutil.StaticRouteSpec{{IPPrefix: "10.0.0.0/8", Nexthop: "172.18.16.99"}})

	rep := analyze(t, a)
	require.Equal(t, 1, rep.Total)
	assert.Equal(t, 1, rep.Unresolved)
	assert.Equal(t, 1, rep.Healthy) // informational, not a warning
	hop := findNextHop(t, rep, "172.18.16.99")
	assert.Equal(t, StatusUnresolved, hop.Status)
	assert.Equal(t, SeverityHealthy, hop.Overall)
	assert.Empty(t, hop.CachedMAC)
}

func TestAnalyze_OutputPort(t *testing.T) {
	a := newAnalyzer(t)
	port := "EXT"
	// Next hop is not inside the LRP subnet, but output_port names the egress.
	testutil.InsertRouterWithStaticRoutes(t, a.NB, "r1", "EXT",
		[]string{"10.0.0.1/24"}, nil,
		[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "192.0.2.1", OutputPort: &port}})
	testutil.InsertMACBinding(t, a.SB, "EXT", "192.0.2.1", "c2:f9:61:ef:64:e0",
		int(refNow.Add(-time.Hour).UnixMilli()))

	rep := analyze(t, a)
	hop := findNextHop(t, rep, "192.0.2.1")
	assert.Equal(t, "EXT", hop.LRPName)
	assert.Equal(t, "c2:f9:61:ef:64:e0", hop.CachedMAC)
	assert.Equal(t, StatusNoAging, hop.Status)
}
