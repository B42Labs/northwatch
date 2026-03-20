package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

func RegisterSB(mux *http.ServeMux, sbClient client.Client) {
	// Tier 1 — highest value tables
	registerTable[sb.Chassis](mux, "/api/v1/sb/chassis", sbClient)
	registerTable[sb.PortBinding](mux, "/api/v1/sb/port-bindings", sbClient)
	registerTable[sb.DatapathBinding](mux, "/api/v1/sb/datapath-bindings", sbClient)

	// Logical flows get special filtering handler
	mux.HandleFunc("GET /api/v1/sb/logical-flows", handleLogicalFlows(sbClient))
	mux.HandleFunc("GET /api/v1/sb/logical-flows/{uuid}", handleGet[sb.LogicalFlow](sbClient))

	// Tier 2
	registerTable[sb.Encap](mux, "/api/v1/sb/encaps", sbClient)
	registerTable[sb.MACBinding](mux, "/api/v1/sb/mac-bindings", sbClient)
	registerTable[sb.FDB](mux, "/api/v1/sb/fdb", sbClient)
	registerTable[sb.MulticastGroup](mux, "/api/v1/sb/multicast-groups", sbClient)
	registerTable[sb.AddressSet](mux, "/api/v1/sb/address-sets", sbClient)
	registerTable[sb.PortGroup](mux, "/api/v1/sb/port-groups", sbClient)
	registerTable[sb.LoadBalancer](mux, "/api/v1/sb/load-balancers", sbClient)
	registerTable[sb.DNS](mux, "/api/v1/sb/dns", sbClient)

	// Tier 3
	registerTable[sb.SBGlobal](mux, "/api/v1/sb/sb-global", sbClient)
	registerTable[sb.Connection](mux, "/api/v1/sb/connections", sbClient)
	registerTable[sb.GatewayChassis](mux, "/api/v1/sb/gateway-chassis", sbClient)
	registerTable[sb.HAChassisGroup](mux, "/api/v1/sb/ha-chassis-groups", sbClient)
	registerTable[sb.HAChassis](mux, "/api/v1/sb/ha-chassis", sbClient)
	registerTable[sb.IPMulticast](mux, "/api/v1/sb/ip-multicast", sbClient)
	registerTable[sb.IGMPGroup](mux, "/api/v1/sb/igmp-groups", sbClient)
	registerTable[sb.ServiceMonitor](mux, "/api/v1/sb/service-monitors", sbClient)
	registerTable[sb.BFD](mux, "/api/v1/sb/bfd", sbClient)
	registerTable[sb.Meter](mux, "/api/v1/sb/meters", sbClient)
	registerTable[sb.Mirror](mux, "/api/v1/sb/mirrors", sbClient)
	registerTable[sb.ChassisPrivate](mux, "/api/v1/sb/chassis-private", sbClient)
	registerTable[sb.ControllerEvent](mux, "/api/v1/sb/controller-events", sbClient)
	registerTable[sb.StaticMACBinding](mux, "/api/v1/sb/static-mac-bindings", sbClient)
	registerTable[sb.LogicalDPGroup](mux, "/api/v1/sb/logical-dp-groups", sbClient)
	registerTable[sb.RBACRole](mux, "/api/v1/sb/rbac-roles", sbClient)
	registerTable[sb.RBACPermission](mux, "/api/v1/sb/rbac-permissions", sbClient)
}

func handleLogicalFlows(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		datapath := r.URL.Query().Get("datapath")
		pipeline := r.URL.Query().Get("pipeline")
		tableIDStr := r.URL.Query().Get("table_id")
		matchFilter := r.URL.Query().Get("match")

		hasFilter := datapath != "" || pipeline != "" || tableIDStr != "" || matchFilter != ""

		var tableID int
		var tableIDSet bool
		if tableIDStr != "" {
			var err error
			tableID, err = strconv.Atoi(tableIDStr)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid table_id: must be integer")
				return
			}
			tableIDSet = true
		}

		if !hasFilter {
			var results []sb.LogicalFlow
			if err := c.List(r.Context(), &results); err != nil {
				api.WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			api.WriteJSON(w, http.StatusOK, api.ModelsToMaps(results))
			return
		}

		var results []sb.LogicalFlow
		err := c.WhereCache(func(f *sb.LogicalFlow) bool {
			if datapath != "" && (f.LogicalDatapath == nil || *f.LogicalDatapath != datapath) {
				return false
			}
			if pipeline != "" && f.Pipeline != pipeline {
				return false
			}
			if tableIDSet && f.TableID != tableID {
				return false
			}
			if matchFilter != "" && !strings.Contains(f.Match, matchFilter) {
				return false
			}
			return true
		}).List(r.Context(), &results)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		api.WriteJSON(w, http.StatusOK, api.ModelsToMaps(results))
	}
}
