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

// topologyIndex bundles the lookup maps derived from a topologyData snapshot
// so that node and edge construction share a single computation.
type topologyIndex struct {
	switchByUUID         map[string]nb.LogicalSwitch
	routerByUUID         map[string]nb.LogicalRouter
	lspToSwitch          map[string]string
	lspNameToUUID        map[string]string
	lrpByName            map[string]nb.LogicalRouterPort
	lrpToRouter          map[string]string
	datapathToEntityUUID map[string]string
	entityChassisCounts  map[string]map[string]int
	entityChassisGroup   map[string]string
	boundChassis         map[string]bool
	gatewayChassis       map[string]bool
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
	input := topologyData{
		switches:     switches,
		routers:      routers,
		lsps:         lsps,
		lrps:         lrps,
		chassisList:  chassisList,
		portBindings: portBindings,
		datapaths:    datapaths,
	}
	idx := buildTopologyIndex(input)
	nodes := buildTopologyNodes(input, idx)
	edges := buildTopologyEdges(input, idx)
	if includeVMs {
		nodes, edges = addVMPorts(input, idx, nodes, edges)
	}
	return TopologyResponse{Nodes: nodes, Edges: edges}
}

// buildTopologyIndex computes the lookup maps shared by node and edge construction.
func buildTopologyIndex(input topologyData) topologyIndex {
	idx := topologyIndex{
		switchByUUID:         make(map[string]nb.LogicalSwitch, len(input.switches)),
		routerByUUID:         make(map[string]nb.LogicalRouter, len(input.routers)),
		lspToSwitch:          make(map[string]string),
		lspNameToUUID:        make(map[string]string, len(input.lsps)),
		lrpByName:            make(map[string]nb.LogicalRouterPort, len(input.lrps)),
		lrpToRouter:          make(map[string]string),
		datapathToEntityUUID: make(map[string]string, len(input.datapaths)),
		entityChassisCounts:  make(map[string]map[string]int),
		entityChassisGroup:   make(map[string]string),
		boundChassis:         make(map[string]bool),
		gatewayChassis:       make(map[string]bool),
	}

	for _, s := range input.switches {
		idx.switchByUUID[s.UUID] = s
		for _, portUUID := range s.Ports {
			idx.lspToSwitch[portUUID] = s.UUID
		}
	}
	for _, r := range input.routers {
		idx.routerByUUID[r.UUID] = r
		for _, portUUID := range r.Ports {
			idx.lrpToRouter[portUUID] = r.UUID
		}
	}
	for _, lsp := range input.lsps {
		idx.lspNameToUUID[lsp.Name] = lsp.UUID
	}
	for _, p := range input.lrps {
		idx.lrpByName[p.Name] = p
	}

	// In OVN, datapath external_ids["logical-switch"] / ["logical-router"]
	// contain the NB entity UUID.
	for _, dp := range input.datapaths {
		if entityUUID, ok := dp.ExternalIDs["logical-switch"]; ok {
			idx.datapathToEntityUUID[dp.UUID] = entityUUID
		} else if entityUUID, ok := dp.ExternalIDs["logical-router"]; ok {
			idx.datapathToEntityUUID[dp.UUID] = entityUUID
		}
	}

	for _, pb := range input.portBindings {
		if pb.Chassis == nil {
			continue
		}
		// Gateway chassis detection is independent of whether the port
		// binding's entity is known to the topology.
		if pb.Type == "chassisredirect" {
			idx.gatewayChassis[*pb.Chassis] = true
		}

		entityUUID := idx.datapathToEntityUUID[pb.Datapath]
		if entityUUID == "" {
			continue
		}
		_, isSwitch := idx.switchByUUID[entityUUID]
		_, isRouter := idx.routerByUUID[entityUUID]
		if !isSwitch && !isRouter {
			continue
		}
		chassisUUID := *pb.Chassis
		if idx.entityChassisCounts[entityUUID] == nil {
			idx.entityChassisCounts[entityUUID] = make(map[string]int)
		}
		idx.entityChassisCounts[entityUUID][chassisUUID]++
		idx.boundChassis[chassisUUID] = true
	}

	// Primary chassis (most port bindings) for convex hull grouping.
	for entityUUID, chassisCounts := range idx.entityChassisCounts {
		var bestChassis string
		var bestCount int
		for chassisUUID, count := range chassisCounts {
			if count > bestCount {
				bestChassis = chassisUUID
				bestCount = count
			}
		}
		idx.entityChassisGroup[entityUUID] = bestChassis
	}

	return idx
}

// buildTopologyNodes builds the node list (switches, routers, chassis).
func buildTopologyNodes(input topologyData, idx topologyIndex) []TopologyNode {
	var nodes []TopologyNode

	for _, s := range input.switches {
		label := s.Name
		if label == "" {
			label = s.UUID[:8]
		}
		nodes = append(nodes, TopologyNode{
			ID:    s.UUID,
			Type:  "switch",
			Label: label,
			Group: idx.entityChassisGroup[s.UUID],
		})
	}

	for _, r := range input.routers {
		label := r.Name
		if label == "" {
			label = r.UUID[:8]
		}
		nodes = append(nodes, TopologyNode{
			ID:    r.UUID,
			Type:  "router",
			Label: label,
			Group: idx.entityChassisGroup[r.UUID],
		})
	}

	// Only include chassis that have port bindings to avoid clutter.
	for _, c := range input.chassisList {
		if !idx.boundChassis[c.UUID] {
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
		if idx.gatewayChassis[c.UUID] {
			meta = map[string]string{"role": "gateway"}
		}
		nodes = append(nodes, TopologyNode{
			ID:       c.UUID,
			Type:     "chassis",
			Label:    label,
			Metadata: meta,
		})
	}

	return nodes
}

// buildTopologyEdges builds the edge list (router-port, patch, binding).
func buildTopologyEdges(input topologyData, idx topologyIndex) []TopologyEdge {
	var edges []TopologyEdge
	edgeSet := make(map[string]bool)

	for _, lsp := range input.lsps {
		switchUUID := idx.lspToSwitch[lsp.UUID]
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
			lrp, ok := idx.lrpByName[routerPortName]
			if !ok {
				continue
			}
			routerUUID := idx.lrpToRouter[lrp.UUID]
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
			peerSwitchUUID := idx.lspToSwitch[idx.lspNameToUUID[peerName]]
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

	// Binding edges: show which chassis hosts each switch/router.
	for entityUUID, chassisCounts := range idx.entityChassisCounts {
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

	return edges
}

// addVMPorts appends VM port nodes and binding edges for VIF port bindings.
func addVMPorts(input topologyData, idx topologyIndex, nodes []TopologyNode, edges []TopologyEdge) ([]TopologyNode, []TopologyEdge) {
	for _, pb := range input.portBindings {
		if pb.Type != "" || pb.Chassis == nil {
			continue
		}
		chassisUUID := *pb.Chassis
		if !idx.boundChassis[chassisUUID] {
			continue
		}

		// Label: prefer IP from neutron:cidrs, else short port name.
		label := pb.LogicalPort
		if len(label) > 8 {
			label = label[:8]
		}
		if cidrs, ok := pb.ExternalIDs["neutron:cidrs"]; ok && cidrs != "" {
			label = cidrs
		}

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

	return nodes, edges
}

func edgeKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}
