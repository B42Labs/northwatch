package handler

import (
	"net/http"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/gateway"
)

// RegisterGatewayHealth registers the gateway (chassisredirect) health endpoint,
// which reports for every distributed gateway port whether the chassis actually
// serving traffic matches the chassis the HA election should have chosen.
func RegisterGatewayHealth(mux *http.ServeMux, nbClient, sbClient client.Client) {
	a := &gateway.Analyzer{NB: nbClient, SB: sbClient}
	mux.HandleFunc("GET /api/v1/topology/gateway", func(w http.ResponseWriter, r *http.Request) {
		report, err := a.Analyze(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		api.WriteJSON(w, http.StatusOK, report)
	})
}
