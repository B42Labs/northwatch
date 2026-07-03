package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/gateway"
	"github.com/b42labs/northwatch/internal/testutil"
)

func TestGatewayHealth(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	// Each call registers a fresh analyzer so the short-TTL memoization cache
	// (exercised directly by gateway.TestAnalyze_Memoized) does not serve a
	// stale snapshot to a later subtest that has changed the DB within the TTL.
	getReport := func(t *testing.T) gateway.Report {
		t.Helper()
		mux := http.NewServeMux()
		RegisterGatewayHealth(mux, nbc, sbc, 60*time.Second)
		req := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/v1/topology/gateway",
			nil,
		)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var rep gateway.Report
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rep))
		return rep
	}

	t.Run("empty", func(t *testing.T) {
		rep := getReport(t)
		assert.Equal(t, 0, rep.Total)
		assert.Empty(t, rep.Gateways)
	})

	t.Run("detects stuck failover", func(t *testing.T) {
		ch1 := testutil.InsertChassis(t, sbc, "netnode-1", "host-1", "10.0.0.1")
		ch2 := testutil.InsertChassis(t, sbc, "netnode-2", "host-2", "10.0.0.2")
		// Both chassis in sync via Chassis_Private (OVN >= 20.06) so both are alive.
		testutil.InsertSBGlobal(t, sbc, 5)
		testutil.InsertChassisPrivate(t, sbc, "netnode-1", &ch1, 5, 1)
		testutil.InsertChassisPrivate(t, sbc, "netnode-2", &ch2, 5, 1)
		grp := testutil.InsertHAChassisGroup(t, sbc, "grp-ext", []testutil.HAChassisEntry{
			{ChassisUUID: ch1, Priority: 30},
			{ChassisUUID: ch2, Priority: 20},
		})
		// Active on the lower-priority chassis -> failover stuck.
		testutil.InsertChassisRedirectBinding(t, sbc, "cr-lrp-ext", &ch2, grp.GroupUUID)
		testutil.InsertGatewayRouter(t, nbc, "router-ext", "lrp-ext", []string{"10.10.141.1/24"}, []string{"10.10.141.24"})

		rep := getReport(t)
		require.Equal(t, 1, rep.Total)
		assert.Equal(t, 1, rep.Error)
		require.Len(t, rep.Gateways, 1)
		gw := rep.Gateways[0]
		assert.Equal(t, gateway.StatusFailoverStuck, gw.Status)
		assert.Equal(t, "netnode-1", gw.DesiredChassis)
		assert.Equal(t, "netnode-2", gw.ActualChassis)
		assert.Contains(t, gw.ServedIPs, "10.10.141.24")
	})
}
