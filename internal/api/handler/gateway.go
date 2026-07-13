package handler

import (
	"net/http"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/gateway"
)

// RegisterGatewayHealth registers the gateway (chassisredirect) health endpoint,
// which reports for every distributed gateway port whether the chassis actually
// serving traffic matches the chassis the HA election should have chosen.
// staleThreshold is how long a chassis may lag the current nb_cfg generation
// before it is treated as not alive for HA election.
func RegisterGatewayHealth(mux *http.ServeMux, nbClient, sbClient client.Client, staleThreshold time.Duration) {
	a := &gateway.Analyzer{NB: nbClient, SB: sbClient, StaleThreshold: staleThreshold}
	mux.HandleFunc("GET /api/v1/topology/gateway", func(w http.ResponseWriter, r *http.Request) {
		report, err := a.Analyze(r.Context())
		if err != nil {
			api.WriteInternalError(w, r, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, report)
	})
}
