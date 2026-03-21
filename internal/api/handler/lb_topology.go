package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// LBBackend represents a single backend target.
type LBBackend struct {
	Address string `json:"address"`
	Status  string `json:"status,omitempty"`
}

// LBVIP represents a VIP with its backends.
type LBVIP struct {
	VIP      string      `json:"vip"`
	Backends []LBBackend `json:"backends"`
}

// LBView represents a load balancer with its VIPs, backends, and health.
type LBView struct {
	UUID        string            `json:"uuid"`
	Name        string            `json:"name"`
	Protocol    *string           `json:"protocol,omitempty"`
	VIPs        []LBVIP           `json:"vips"`
	Routers     []string          `json:"routers"`
	Switches    []string          `json:"switches"`
	ExternalIDs map[string]string `json:"external_ids,omitempty"`
}

// LBTopologyResponse groups load balancers with health info.
type LBTopologyResponse struct {
	Total         int      `json:"total"`
	LoadBalancers []LBView `json:"load_balancers"`
}

// RegisterLBTopology registers the load balancer topology endpoint.
func RegisterLBTopology(mux *http.ServeMux, nbClient, sbClient client.Client) {
	mux.HandleFunc("GET /api/v1/topology/load-balancers", handleLBTopology(nbClient, sbClient))
}

func handleLBTopology(nbClient, sbClient client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var lbs []nb.LoadBalancer
		if err := nbClient.List(ctx, &lbs); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list load balancers")
			return
		}

		var monitors []sb.ServiceMonitor
		_ = sbClient.List(ctx, &monitors)

		monitorStatus := make(map[string]string)
		for _, m := range monitors {
			key := m.IP
			if m.Port > 0 {
				key = fmt.Sprintf("%s:%d", m.IP, m.Port)
			}
			if m.Status != nil {
				monitorStatus[key] = string(*m.Status)
			}
		}

		var routers []nb.LogicalRouter
		_ = nbClient.List(ctx, &routers)
		var switches []nb.LogicalSwitch
		_ = nbClient.List(ctx, &switches)

		lbRouters := make(map[string][]string)
		for _, router := range routers {
			for _, lbUUID := range router.LoadBalancer {
				lbRouters[lbUUID] = append(lbRouters[lbUUID], router.Name)
			}
		}
		lbSwitches := make(map[string][]string)
		for _, sw := range switches {
			for _, lbUUID := range sw.LoadBalancer {
				lbSwitches[lbUUID] = append(lbSwitches[lbUUID], sw.Name)
			}
		}

		resp := LBTopologyResponse{
			Total:         len(lbs),
			LoadBalancers: make([]LBView, 0, len(lbs)),
		}

		for _, lb := range lbs {
			view := LBView{
				UUID:        lb.UUID,
				Name:        lb.Name,
				Protocol:    lb.Protocol,
				ExternalIDs: lb.ExternalIDs,
				Routers:     lbRouters[lb.UUID],
				Switches:    lbSwitches[lb.UUID],
			}
			if view.Routers == nil {
				view.Routers = []string{}
			}
			if view.Switches == nil {
				view.Switches = []string{}
			}

			view.VIPs = make([]LBVIP, 0)
			for vip, backends := range lb.Vips {
				lbVIP := LBVIP{VIP: vip, Backends: []LBBackend{}}
				for _, backend := range strings.Split(backends, ",") {
					backend = strings.TrimSpace(backend)
					if backend == "" {
						continue
					}
					b := LBBackend{Address: backend}
					if status, ok := monitorStatus[backend]; ok {
						b.Status = status
					}
					lbVIP.Backends = append(lbVIP.Backends, b)
				}
				view.VIPs = append(view.VIPs, lbVIP)
			}

			resp.LoadBalancers = append(resp.LoadBalancers, view)
		}

		api.WriteJSON(w, http.StatusOK, resp)
	}
}
