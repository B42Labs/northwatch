package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
)

func TestNATTopology(t *testing.T) {
	nbClient := setupNBTestClient(t)
	ctx := context.Background()

	mux := http.NewServeMux()
	RegisterNATTopology(mux, nbClient)

	t.Run("empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/nat", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp NATTopologyResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Total)
		assert.Empty(t, resp.SNAT)
		assert.Empty(t, resp.DNAT)
		assert.Empty(t, resp.DNATAndSNAT)
	})

	t.Run("groups by type with router context", func(t *testing.T) {
		snatNamed := "nat_snat"
		dnatNamed := "nat_dnat"
		bothNamed := "nat_both"
		snat := &nb.NAT{UUID: snatNamed, Type: "snat", ExternalIP: "1.1.1.1", LogicalIP: "10.0.0.0/24", ExternalIDs: map[string]string{}}
		dnat := &nb.NAT{UUID: dnatNamed, Type: "dnat", ExternalIP: "2.2.2.2", LogicalIP: "10.0.0.10", ExternalIDs: map[string]string{}}
		both := &nb.NAT{UUID: bothNamed, Type: "dnat_and_snat", ExternalIP: "3.3.3.3", LogicalIP: "10.0.0.20", ExternalIDs: map[string]string{}}
		router := &nb.LogicalRouter{
			Name:        "edge-router",
			Nat:         []string{snatNamed, dnatNamed, bothNamed},
			ExternalIDs: map[string]string{},
		}

		var allOps []ovsdb.Operation
		for _, m := range []model.Model{snat, dnat, both, router} {
			ops, err := nbClient.Create(m)
			require.NoError(t, err)
			allOps = append(allOps, ops...)
		}
		reply, err := nbClient.Transact(ctx, allOps...)
		require.NoError(t, err)
		_, err = ovsdb.CheckOperationResults(reply, allOps)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/nat", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp NATTopologyResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		assert.Equal(t, 3, resp.Total)
		require.Len(t, resp.SNAT, 1)
		require.Len(t, resp.DNAT, 1)
		require.Len(t, resp.DNATAndSNAT, 1)
		assert.Equal(t, "edge-router", resp.SNAT[0].RouterName)
		assert.Equal(t, "1.1.1.1", resp.SNAT[0].ExternalIP)
		assert.Equal(t, "10.0.0.10", resp.DNAT[0].LogicalIP)
		assert.Equal(t, "edge-router", resp.DNATAndSNAT[0].RouterName)
	})
}
