package handler

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// TopologyNode represents a node in the network topology graph.
type TopologyNode struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Label    string            `json:"label"`
	Group    string            `json:"group,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TopologyEdge represents a connection between two nodes.
type TopologyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// TopologyResponse is the JSON response for GET /api/v1/topology.
type TopologyResponse struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// RegisterTopology registers the topology endpoint.
func RegisterTopology(mux *http.ServeMux, nbClient, sbClient client.Client) {
	mux.HandleFunc("GET /api/v1/topology", handleTopology(nbClient, sbClient))
}

// topologyData holds the raw OVSDB rows needed to build a topology graph.
type topologyData struct {
	switches     []nb.LogicalSwitch
	routers      []nb.LogicalRouter
	lsps         []nb.LogicalSwitchPort
	lrps         []nb.LogicalRouterPort
	chassisList  []sb.Chassis
	portBindings []sb.PortBinding
	datapaths    []sb.DatapathBinding
}

// fetchTopologyData fetches all NB and SB tables needed for buildTopology in
// parallel and returns them grouped together. If any List call fails the
// returned error names the failing source.
func fetchTopologyData(ctx context.Context, nbClient, sbClient client.Client) (*topologyData, error) {
	var data topologyData
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := nbClient.List(gctx, &data.switches); err != nil {
			return fmt.Errorf("logical switches: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := nbClient.List(gctx, &data.routers); err != nil {
			return fmt.Errorf("logical routers: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := nbClient.List(gctx, &data.lsps); err != nil {
			return fmt.Errorf("logical switch ports: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := nbClient.List(gctx, &data.lrps); err != nil {
			return fmt.Errorf("logical router ports: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := sbClient.List(gctx, &data.chassisList); err != nil {
			return fmt.Errorf("chassis: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := sbClient.List(gctx, &data.portBindings); err != nil {
			return fmt.Errorf("port bindings: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := sbClient.List(gctx, &data.datapaths); err != nil {
			return fmt.Errorf("datapath bindings: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return &data, nil
}

func handleTopology(nbClient, sbClient client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fetchTopologyData(r.Context(), nbClient, sbClient)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to fetch topology data: "+err.Error())
			return
		}

		// Build topology
		includeVMs := r.URL.Query().Get("vms") == "true"
		resp := buildTopology(data.switches, data.routers, data.lsps, data.lrps, data.chassisList, data.portBindings, data.datapaths, includeVMs)

		if r.URL.Query().Get("format") == "download" {
			w.Header().Set("Content-Disposition", "attachment; filename=topology.json")
		}

		api.WriteJSON(w, http.StatusOK, resp)
	}
}

func buildTopology(
	switches []nb.LogicalSwitch,
	routers []nb.LogicalRouter,
	lsps []nb.LogicalSwitchPort,
	lrps []nb.LogicalRouterPort,
	chassisList []sb.Chassis,
	portBindings []sb.PortBinding,
	datapaths []sb.DatapathBinding,
	includeVMs bool,
) TopologyResponse {
	var nodes []TopologyNode
	var edges []TopologyEdge

	// Build lookup maps
	switchByUUID := make(map[string]nb.LogicalSwitch, len(switches))
	for _, s := range switches {
		switchByUUID[s.UUID] = s
	}

	routerByUUID := make(map[string]nb.LogicalRouter, len(routers))
	for _, r := range routers {
		routerByUUID[r.UUID] = r
	}

	// Map LSP UUID → parent switch UUID
	lspToSwitch := make(map[string]string)
	for _, s := range switches {
		for _, portUUID := range s.Ports {
			lspToSwitch[portUUID] = s.UUID
		}
	}

	// Map LSP name → UUID for O(1) lookup
	lspNameToUUID := make(map[string]string, len(lsps))
	for _, lsp := range lsps {
		lspNameToUUID[lsp.Name] = lsp.UUID
	}

	// Map LRP name → LRP and LRP UUID → parent router UUID
	lrpByName := make(map[string]nb.LogicalRouterPort, len(lrps))
	lrpToRouter := make(map[string]string)
	for _, r := range routers {
		for _, portUUID := range r.Ports {
			lrpToRouter[portUUID] = r.UUID
		}
	}
	for _, p := range lrps {
		lrpByName[p.Name] = p
	}

	// Map datapath UUID → entity UUID via external_ids
	// In OVN, external_ids["logical-switch"] and ["logical-router"] contain the NB entity UUID
	datapathToEntityUUID := make(map[string]string, len(datapaths))
	for _, dp := range datapaths {
		if entityUUID, ok := dp.ExternalIDs["logical-switch"]; ok {
			datapathToEntityUUID[dp.UUID] = entityUUID
		} else if entityUUID, ok := dp.ExternalIDs["logical-router"]; ok {
			datapathToEntityUUID[dp.UUID] = entityUUID
		}
	}

	// Track all chassis bindings per entity via datapath mapping (covers both switches and routers)
	entityChassisCounts := make(map[string]map[string]int) // entity UUID → chassis UUID → port count
	for _, pb := range portBindings {
		if pb.Chassis == nil {
			continue
		}
		chassisUUID := *pb.Chassis
		entityUUID := datapathToEntityUUID[pb.Datapath]
		if entityUUID == "" {
			continue
		}
		// Verify entity exists in our topology
		_, isSwitch := switchByUUID[entityUUID]
		_, isRouter := routerByUUID[entityUUID]
		if !isSwitch && !isRouter {
			continue
		}
		if entityChassisCounts[entityUUID] == nil {
			entityChassisCounts[entityUUID] = make(map[string]int)
		}
		entityChassisCounts[entityUUID][chassisUUID]++
	}

	// Primary chassis (most port bindings) for convex hull grouping
	entityChassisGroup := make(map[string]string)
	for entityUUID, chassisCounts := range entityChassisCounts {
		var bestChassis string
		var bestCount int
		for chassisUUID, count := range chassisCounts {
			if count > bestCount {
				bestChassis = chassisUUID
				bestCount = count
			}
		}
		entityChassisGroup[entityUUID] = bestChassis
	}

	// Switch nodes
	for _, s := range switches {
		label := s.Name
		if label == "" {
			label = s.UUID[:8]
		}
		nodes = append(nodes, TopologyNode{
			ID:    s.UUID,
			Type:  "switch",
			Label: label,
			Group: entityChassisGroup[s.UUID],
		})
	}

	// Router nodes
	for _, r := range routers {
		label := r.Name
		if label == "" {
			label = r.UUID[:8]
		}
		nodes = append(nodes, TopologyNode{
			ID:    r.UUID,
			Type:  "router",
			Label: label,
			Group: entityChassisGroup[r.UUID],
		})
	}

	// Chassis nodes — only include chassis that have port bindings to avoid clutter
	boundChassis := make(map[string]bool)
	for _, counts := range entityChassisCounts {
		for chassisUUID := range counts {
			boundChassis[chassisUUID] = true
		}
	}

	// Detect gateway chassis (those hosting chassisredirect port bindings)
	gatewayChassis := make(map[string]bool)
	for _, pb := range portBindings {
		if pb.Type == "chassisredirect" && pb.Chassis != nil {
			gatewayChassis[*pb.Chassis] = true
		}
	}

	for _, c := range chassisList {
		if !boundChassis[c.UUID] {
			continue
		}
		label := c.Name
		if label == "" {
			label = c.Hostname
		}
		if label == "" {
			label = c.UUID[:8]
		}
		var meta map[string]string
		if gatewayChassis[c.UUID] {
			meta = map[string]string{"role": "gateway"}
		}
		nodes = append(nodes, TopologyNode{
			ID:       c.UUID,
			Type:     "chassis",
			Label:    label,
			Metadata: meta,
		})
	}

	// Track edges to avoid duplicates
	edgeSet := make(map[string]bool)

	// Edges from LSPs
	for _, lsp := range lsps {
		switchUUID := lspToSwitch[lsp.UUID]
		if switchUUID == "" {
			continue
		}

		switch lsp.Type {
		case "router":
			// router-port type: options["router-port"] → LRP name → router UUID
			routerPortName := lsp.Options["router-port"]
			if routerPortName == "" {
				continue
			}
			lrp, ok := lrpByName[routerPortName]
			if !ok {
				continue
			}
			routerUUID := lrpToRouter[lrp.UUID]
			if routerUUID == "" {
				continue
			}
			key := edgeKey(switchUUID, routerUUID)
			if !edgeSet[key] {
				edgeSet[key] = true
				edges = append(edges, TopologyEdge{
					Source: switchUUID,
					Target: routerUUID,
					Type:   "router-port",
				})
			}

		case "patch":
			// patch type: options["peer"] → another LSP name → its parent switch
			peerName := lsp.Options["peer"]
			if peerName == "" {
				continue
			}
			peerSwitchUUID := lspToSwitch[lspNameToUUID[peerName]]
			if peerSwitchUUID == "" {
				continue
			}
			key := edgeKey(switchUUID, peerSwitchUUID)
			if !edgeSet[key] {
				edgeSet[key] = true
				edges = append(edges, TopologyEdge{
					Source: switchUUID,
					Target: peerSwitchUUID,
					Type:   "patch",
				})
			}
		}
	}

	// Binding edges: show which chassis hosts each switch/router
	for entityUUID, chassisCounts := range entityChassisCounts {
		for chassisUUID := range chassisCounts {
			key := edgeKey(entityUUID, chassisUUID)
			if !edgeSet[key] {
				edgeSet[key] = true
				edges = append(edges, TopologyEdge{
					Source: entityUUID,
					Target: chassisUUID,
					Type:   "binding",
				})
			}
		}
	}

	// VM port nodes: VIF ports (type="") bound to a chassis
	if includeVMs {
		for _, pb := range portBindings {
			if pb.Type != "" || pb.Chassis == nil {
				continue
			}
			chassisUUID := *pb.Chassis
			if !boundChassis[chassisUUID] {
				continue
			}

			// Label: prefer IP from neutron:cidrs, else short port name
			label := pb.LogicalPort
			if len(label) > 8 {
				label = label[:8]
			}
			if cidrs, ok := pb.ExternalIDs["neutron:cidrs"]; ok && cidrs != "" {
				label = cidrs
			}

			// Extract neutron metadata
			meta := make(map[string]string)
			for _, key := range []string{
				"neutron:device_id", "neutron:device_owner",
				"neutron:host_id", "neutron:cidrs",
				"neutron:port_fip", "neutron:network_name",
			} {
				if v, ok := pb.ExternalIDs[key]; ok {
					meta[key[len("neutron:"):]] = v
				}
			}
			if len(pb.MAC) > 0 {
				meta["mac"] = pb.MAC[0]
			}
			if pb.Up != nil {
				if *pb.Up {
					meta["up"] = "true"
				} else {
					meta["up"] = "false"
				}
			}

			nodes = append(nodes, TopologyNode{
				ID:       pb.UUID,
				Type:     "vm-port",
				Label:    label,
				Group:    chassisUUID,
				Metadata: meta,
			})

			edges = append(edges, TopologyEdge{
				Source: pb.UUID,
				Target: chassisUUID,
				Type:   "vm-binding",
			})
		}
	}

	return TopologyResponse{Nodes: nodes, Edges: edges}
}

func edgeKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}
