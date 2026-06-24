package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/router"
	"github.com/b42labs/northwatch/internal/testutil"
)

func TestNextHopMAC(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	mux := http.NewServeMux()
	RegisterNextHopMAC(mux, nbc, sbc)

	getReport := func(t *testing.T) router.Report {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/debug/nexthop-mac", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var rep router.Report
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rep))
		return rep
	}

	t.Run("empty", func(t *testing.T) {
		rep := getReport(t)
		assert.Equal(t, 0, rep.Total)
		assert.Empty(t, rep.NextHops)
	})

	t.Run("flags next hop without aging", func(t *testing.T) {
		testutil.InsertRouterWithStaticRoutes(t, nbc, "router-ext", "EXT",
			[]string{"172.18.16.1/24"}, nil,
			[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "172.18.16.1"}})
		testutil.InsertMACBinding(t, sbc, "EXT", "172.18.16.1", "c2:f9:61:ef:64:e0", 0)

		rep := getReport(t)
		require.Equal(t, 1, rep.Total)
		assert.Equal(t, 1, rep.NoAging)
		require.Len(t, rep.NextHops, 1)
		assert.Equal(t, router.StatusNoAging, rep.NextHops[0].Status)
		assert.NotEmpty(t, rep.NextHops[0].MACBindingUUID) // available for destroy remediation
	})
}
