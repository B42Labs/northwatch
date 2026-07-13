package handler

import (
	"net/http"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovscorrelate"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
)

// RegisterOVSCorrelation registers the per-chassis OVS↔OVN correlation endpoint.
// It reads a live OVS interface from the chassis's monitored cache and resolves
// its external_ids:iface-id against the default cluster's Southbound cache
// (sbClient) to surface the bound Port_Binding. Because only the default cluster
// wires an OVS pool, the endpoint is default-cluster-only.
func RegisterOVSCorrelation(mux *http.ServeMux, pool *ovndb.OVSPool, sbClient client.Client) {
	cor := &ovscorrelate.Correlator{SB: sbClient}

	mux.HandleFunc("GET /api/v1/ovs/{chassis}/interface/{uuid}/correlation", func(w http.ResponseWriter, r *http.Request) {
		c, ok := resolveOVSChassis(w, r, pool)
		if !ok {
			return
		}
		uuid := r.PathValue("uuid")

		var ifaces []vs.Interface
		if err := c.WhereCache(func(i *vs.Interface) bool {
			return i.UUID == uuid
		}).List(r.Context(), &ifaces); err != nil {
			api.WriteInternalError(w, r, err)
			return
		}
		if len(ifaces) == 0 {
			api.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		iface := ifaces[0]

		result, err := cor.ForInterface(r.Context(), ovscorrelate.LiveInterface{
			SystemID:  r.PathValue("chassis"),
			IfaceID:   iface.ExternalIDs["iface-id"],
			LinkState: ovndb.DerefStr(iface.LinkState),
			Error:     ovndb.DerefStr(iface.Error),
		})
		if err != nil {
			api.WriteInternalError(w, r, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	})
}
