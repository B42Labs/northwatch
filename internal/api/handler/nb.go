package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

func RegisterNB(mux *http.ServeMux, nbClient client.Client) {
	// Tier 1 — highest value tables
	registerTable[nb.LogicalSwitch](mux, "/api/v1/nb/logical-switches", nbClient)
	registerTable[nb.LogicalSwitchPort](mux, "/api/v1/nb/logical-switch-ports", nbClient)
	registerTable[nb.LogicalRouter](mux, "/api/v1/nb/logical-routers", nbClient)
	registerTable[nb.LogicalRouterPort](mux, "/api/v1/nb/logical-router-ports", nbClient)

	// Tier 2
	registerTable[nb.ACL](mux, "/api/v1/nb/acls", nbClient)
	registerTable[nb.NAT](mux, "/api/v1/nb/nats", nbClient)
	registerTable[nb.AddressSet](mux, "/api/v1/nb/address-sets", nbClient)
	registerTable[nb.PortGroup](mux, "/api/v1/nb/port-groups", nbClient)
	registerTable[nb.LoadBalancer](mux, "/api/v1/nb/load-balancers", nbClient)
	registerTable[nb.LoadBalancerGroup](mux, "/api/v1/nb/load-balancer-groups", nbClient)
	registerTable[nb.LogicalRouterPolicy](mux, "/api/v1/nb/logical-router-policies", nbClient)
	registerTable[nb.LogicalRouterStaticRoute](mux, "/api/v1/nb/logical-router-static-routes", nbClient)
	registerTable[nb.DHCPOptions](mux, "/api/v1/nb/dhcp-options", nbClient)

	// Tier 3
	registerTable[nb.NBGlobal](mux, "/api/v1/nb/nb-global", nbClient)
	registerTable[nb.Connection](mux, "/api/v1/nb/connections", nbClient)
	registerTable[nb.DNS](mux, "/api/v1/nb/dns", nbClient)
	registerTable[nb.GatewayChassis](mux, "/api/v1/nb/gateway-chassis", nbClient)
	registerTable[nb.HAChassisGroup](mux, "/api/v1/nb/ha-chassis-groups", nbClient)
	registerTable[nb.HAChassis](mux, "/api/v1/nb/ha-chassis", nbClient)
	registerTable[nb.Meter](mux, "/api/v1/nb/meters", nbClient)
	registerTable[nb.QoS](mux, "/api/v1/nb/qos", nbClient)
	registerTable[nb.BFD](mux, "/api/v1/nb/bfd", nbClient)
	registerTable[nb.Copp](mux, "/api/v1/nb/copp", nbClient)
	registerTable[nb.Mirror](mux, "/api/v1/nb/mirrors", nbClient)
	registerTable[nb.ForwardingGroup](mux, "/api/v1/nb/forwarding-groups", nbClient)
	registerTable[nb.StaticMACBinding](mux, "/api/v1/nb/static-mac-bindings", nbClient)
	registerTable[nb.LoadBalancerHealthCheck](mux, "/api/v1/nb/load-balancer-health-checks", nbClient)
}

// registerTable registers list and get-by-uuid handlers for an OVSDB model type.
func registerTable[T any](mux *http.ServeMux, basePath string, c client.Client) {
	mux.HandleFunc("GET "+basePath, handleList[T](c))
	mux.HandleFunc("GET "+basePath+"/{uuid}", handleGet[T](c))
}

func handleList[T any](c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var results []T
		if err := c.List(r.Context(), &results); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		api.WriteJSON(w, http.StatusOK, api.ModelsToMaps(results))
	}
}

func handleGet[T any](c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := r.PathValue("uuid")
		if uuid == "" {
			api.WriteError(w, http.StatusBadRequest, "uuid is required")
			return
		}

		var results []T
		if err := c.WhereCache(func(m *T) bool {
			return getUUID(m) == uuid
		}).List(r.Context(), &results); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if len(results) == 0 {
			api.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		api.WriteJSON(w, http.StatusOK, api.ModelToMap(results[0]))
	}
}
