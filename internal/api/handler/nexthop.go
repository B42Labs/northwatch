package handler

import (
	"net/http"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/router"
)

// RegisterNextHopMAC registers the logical-router next-hop MAC health endpoint,
// which correlates each static route's next hop with the cached SB MAC_Binding /
// Static_MAC_Binding and flags entries that can go stale because ARP-cache aging
// (mac_binding_age_threshold) is not configured.
func RegisterNextHopMAC(mux *http.ServeMux, nbClient, sbClient client.Client) {
	a := &router.Analyzer{NB: nbClient, SB: sbClient}
	mux.HandleFunc("GET /api/v1/debug/nexthop-mac", func(w http.ResponseWriter, r *http.Request) {
		report, err := a.Analyze(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		api.WriteJSON(w, http.StatusOK, report)
	})
}
