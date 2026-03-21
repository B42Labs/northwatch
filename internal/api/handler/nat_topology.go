package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// NATMapping represents a single NAT entry with its router context.
type NATMapping struct {
	UUID        string `json:"uuid"`
	Type        string `json:"type"`
	ExternalIP  string `json:"external_ip"`
	LogicalIP   string `json:"logical_ip"`
	RouterUUID  string `json:"router_uuid"`
	RouterName  string `json:"router_name"`
	LogicalPort string `json:"logical_port,omitempty"`
	ExternalMAC string `json:"external_mac,omitempty"`
}

// NATTopologyResponse groups NAT rules by type with router context.
type NATTopologyResponse struct {
	Total       int          `json:"total"`
	SNAT        []NATMapping `json:"snat"`
	DNAT        []NATMapping `json:"dnat"`
	DNATAndSNAT []NATMapping `json:"dnat_and_snat"`
}

// RegisterNATTopology registers the NAT topology endpoint.
func RegisterNATTopology(mux *http.ServeMux, nbClient client.Client) {
	mux.HandleFunc("GET /api/v1/topology/nat", handleNATTopology(nbClient))
}

func handleNATTopology(nbClient client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var routers []nb.LogicalRouter
		if err := nbClient.List(ctx, &routers); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list logical routers")
			return
		}

		var nats []nb.NAT
		if err := nbClient.List(ctx, &nats); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list NAT rules")
			return
		}

		natToRouter := make(map[string]nb.LogicalRouter)
		for _, router := range routers {
			for _, natUUID := range router.Nat {
				natToRouter[natUUID] = router
			}
		}

		resp := NATTopologyResponse{
			Total:       len(nats),
			SNAT:        []NATMapping{},
			DNAT:        []NATMapping{},
			DNATAndSNAT: []NATMapping{},
		}

		for _, n := range nats {
			router := natToRouter[n.UUID]
			routerName := router.Name
			if routerName == "" && router.UUID != "" {
				routerName = router.UUID
			}

			mapping := NATMapping{
				UUID:       n.UUID,
				Type:       n.Type,
				ExternalIP: n.ExternalIP,
				LogicalIP:  n.LogicalIP,
				RouterUUID: router.UUID,
				RouterName: routerName,
			}
			if n.LogicalPort != nil {
				mapping.LogicalPort = *n.LogicalPort
			}
			if n.ExternalMAC != nil {
				mapping.ExternalMAC = *n.ExternalMAC
			}

			switch n.Type {
			case "snat":
				resp.SNAT = append(resp.SNAT, mapping)
			case "dnat":
				resp.DNAT = append(resp.DNAT, mapping)
			case "dnat_and_snat":
				resp.DNATAndSNAT = append(resp.DNATAndSNAT, mapping)
			}
		}

		api.WriteJSON(w, http.StatusOK, resp)
	}
}
