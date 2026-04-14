package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

func TestLBTopology(t *testing.T) {
	nbClient := setupNBTestClient(t)
	sbClient := setupSBTestClient(t)
	ctx := context.Background()

	mux := http.NewServeMux()
	RegisterLBTopology(mux, nbClient, sbClient)

	t.Run("empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/load-balancers", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp LBTopologyResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Total)
		assert.Empty(t, resp.LoadBalancers)
	})

	t.Run("with backends and attachments", func(t *testing.T) {
		tcp := "tcp"
		lbUUIDName := "lb_test"
		lb := &nb.LoadBalancer{
			UUID:     lbUUIDName,
			Name:     "test-lb",
			Protocol: &tcp,
			Vips: map[string]string{
				"10.0.0.1:80": "192.168.1.10:8080,192.168.1.11:8080",
			},
			ExternalIDs: map[string]string{"managed-by": "test"},
		}
		router := &nb.LogicalRouter{
			Name:         "test-rtr",
			LoadBalancer: []string{lbUUIDName},
			ExternalIDs:  map[string]string{},
		}
		sw := &nb.LogicalSwitch{
			Name:         "test-sw",
			LoadBalancer: []string{lbUUIDName},
			ExternalIDs:  map[string]string{},
		}

		lbOps, err := nbClient.Create(lb)
		require.NoError(t, err)
		rOps, err := nbClient.Create(router)
		require.NoError(t, err)
		swOps, err := nbClient.Create(sw)
		require.NoError(t, err)

		allOps := append(lbOps, rOps...)
		allOps = append(allOps, swOps...)

		reply, err := nbClient.Transact(ctx, allOps...)
		require.NoError(t, err)
		_, err = ovsdb.CheckOperationResults(reply, allOps)
		require.NoError(t, err)
		lbUUID := reply[0].UUID.GoUUID

		// Insert a service monitor that marks one backend as up.
		online := sb.ServiceMonitorStatusOnline
		monitor := &sb.ServiceMonitor{
			IP:          "192.168.1.10",
			Port:        8080,
			Status:      &online,
			ExternalIDs: map[string]string{},
			Options:     map[string]string{},
		}
		mOps, err := sbClient.Create(monitor)
		require.NoError(t, err)
		mReply, err := sbClient.Transact(ctx, mOps...)
		require.NoError(t, err)
		_, err = ovsdb.CheckOperationResults(mReply, mOps)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/load-balancers", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp LBTopologyResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		require.Equal(t, 1, resp.Total)
		require.Len(t, resp.LoadBalancers, 1)
		view := resp.LoadBalancers[0]
		assert.Equal(t, lbUUID, view.UUID)
		assert.Equal(t, "test-lb", view.Name)
		require.NotNil(t, view.Protocol)
		assert.Equal(t, "tcp", *view.Protocol)
		assert.Equal(t, []string{"test-rtr"}, view.Routers)
		assert.Equal(t, []string{"test-sw"}, view.Switches)

		require.Len(t, view.VIPs, 1)
		vip := view.VIPs[0]
		assert.Equal(t, "10.0.0.1:80", vip.VIP)
		require.Len(t, vip.Backends, 2)

		// Order is map-iteration-dependent. Look up by address.
		backendByAddr := map[string]LBBackend{}
		for _, b := range vip.Backends {
			backendByAddr[b.Address] = b
		}
		assert.Equal(t, "online", backendByAddr["192.168.1.10:8080"].Status)
		assert.Empty(t, backendByAddr["192.168.1.11:8080"].Status, "monitor only covers .10")
	})
}
