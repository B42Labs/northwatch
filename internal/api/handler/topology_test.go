package handler

import (
	"testing"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/stretchr/testify/assert"
)

func TestBuildTopology_Basic(t *testing.T) {
	switches := []nb.LogicalSwitch{
		{UUID: "ls-1", Name: "switch1", Ports: []string{"lsp-1", "lsp-2"}},
	}
	routers := []nb.LogicalRouter{
		{UUID: "lr-1", Name: "router1", Ports: []string{"lrp-1"}},
	}
	lsps := []nb.LogicalSwitchPort{
		{UUID: "lsp-1", Name: "lsp-to-router", Type: "router", Options: map[string]string{"router-port": "lrp-1-name"}},
		{UUID: "lsp-2", Name: "regular-port", Type: ""},
	}
	lrps := []nb.LogicalRouterPort{
		{UUID: "lrp-1", Name: "lrp-1-name"},
	}
	chassisList := []sb.Chassis{
		{UUID: "ch-1", Name: "host1", Hostname: "host1.example.com"},
	}

	resp := buildTopology(switches, routers, lsps, lrps, chassisList, nil, nil, false)

	// Should have 2 nodes: 1 switch + 1 router (chassis without bindings excluded)
	assert.Len(t, resp.Nodes, 2)

	// Verify node types
	types := make(map[string]string)
	for _, n := range resp.Nodes {
		types[n.ID] = n.Type
	}
	assert.Equal(t, "switch", types["ls-1"])
	assert.Equal(t, "router", types["lr-1"])

	// Should have 1 edge: switch ↔ router
	assert.Len(t, resp.Edges, 1)
	assert.Equal(t, "router-port", resp.Edges[0].Type)
}

func TestBuildTopology_PatchPorts(t *testing.T) {
	switches := []nb.LogicalSwitch{
		{UUID: "ls-1", Name: "switch1", Ports: []string{"lsp-1"}},
		{UUID: "ls-2", Name: "switch2", Ports: []string{"lsp-2"}},
	}
	lsps := []nb.LogicalSwitchPort{
		{UUID: "lsp-1", Name: "patch-to-2", Type: "patch", Options: map[string]string{"peer": "patch-to-1"}},
		{UUID: "lsp-2", Name: "patch-to-1", Type: "patch", Options: map[string]string{"peer": "patch-to-2"}},
	}

	resp := buildTopology(switches, nil, lsps, nil, nil, nil, nil, false)

	// Should have 2 switch nodes
	assert.Len(t, resp.Nodes, 2)

	// Should have 1 patch edge (deduplicated)
	assert.Len(t, resp.Edges, 1)
	assert.Equal(t, "patch", resp.Edges[0].Type)
}

func TestBuildTopology_Empty(t *testing.T) {
	resp := buildTopology(nil, nil, nil, nil, nil, nil, nil, false)
	assert.Empty(t, resp.Nodes)
	assert.Empty(t, resp.Edges)
}

func TestBuildTopology_ChassisGrouping(t *testing.T) {
	switches := []nb.LogicalSwitch{
		{UUID: "ls-1", Name: "switch1", Ports: []string{"lsp-1"}},
	}
	lsps := []nb.LogicalSwitchPort{
		{UUID: "lsp-1", Name: "port1", Type: ""},
	}
	chassisList := []sb.Chassis{
		{UUID: "ch-1", Name: "host1"},
	}
	chassisUUID := "ch-1"
	portBindings := []sb.PortBinding{
		{UUID: "pb-1", LogicalPort: "port1", Chassis: &chassisUUID, Datapath: "dp-1"},
	}
	datapaths := []sb.DatapathBinding{
		{UUID: "dp-1", ExternalIDs: map[string]string{"logical-switch": "ls-1"}},
	}

	resp := buildTopology(switches, nil, lsps, nil, chassisList, portBindings, datapaths, false)

	// Switch node should have chassis group
	for _, n := range resp.Nodes {
		if n.ID == "ls-1" {
			assert.Equal(t, "ch-1", n.Group)
		}
	}

	// Should have a binding edge from switch to chassis
	var hasBindingEdge bool
	for _, e := range resp.Edges {
		if e.Type == "binding" {
			hasBindingEdge = true
			assert.Equal(t, "ls-1", e.Source)
			assert.Equal(t, "ch-1", e.Target)
		}
	}
	assert.True(t, hasBindingEdge, "expected binding edge between switch and chassis")
}

func TestBuildTopology_RouterChassisGrouping(t *testing.T) {
	routers := []nb.LogicalRouter{
		{UUID: "lr-1", Name: "router1", Ports: []string{"lrp-1"}},
	}
	lrps := []nb.LogicalRouterPort{
		{UUID: "lrp-1", Name: "lrp1"},
	}
	chassisList := []sb.Chassis{
		{UUID: "ch-1", Name: "host1"},
	}
	chassisUUID := "ch-1"
	portBindings := []sb.PortBinding{
		{UUID: "pb-1", LogicalPort: "cr-lrp1", Chassis: &chassisUUID, Datapath: "dp-1", Type: "chassisredirect"},
	}
	datapaths := []sb.DatapathBinding{
		{UUID: "dp-1", ExternalIDs: map[string]string{"logical-router": "lr-1"}},
	}

	resp := buildTopology(nil, routers, nil, lrps, chassisList, portBindings, datapaths, false)

	// Router node should have chassis group
	for _, n := range resp.Nodes {
		if n.ID == "lr-1" {
			assert.Equal(t, "ch-1", n.Group, "router should be assigned to chassis")
		}
	}

	// Should have a binding edge from router to chassis
	var hasBindingEdge bool
	for _, e := range resp.Edges {
		if e.Type == "binding" {
			hasBindingEdge = true
		}
	}
	assert.True(t, hasBindingEdge, "expected binding edge between router and chassis")
}

func TestBuildTopology_VMPorts(t *testing.T) {
	switches := []nb.LogicalSwitch{
		{UUID: "ls-1", Name: "switch1", Ports: []string{"lsp-1"}},
	}
	lsps := []nb.LogicalSwitchPort{
		{UUID: "lsp-1", Name: "port1", Type: ""},
	}
	chassisList := []sb.Chassis{
		{UUID: "ch-1", Name: "host1"},
	}
	chassisUUID := "ch-1"
	up := true
	portBindings := []sb.PortBinding{
		{UUID: "pb-1", LogicalPort: "port1", Chassis: &chassisUUID, Datapath: "dp-1", Type: "patch"},
		{
			UUID:        "pb-vm-1",
			LogicalPort: "vm-port-1",
			Type:        "",
			Chassis:     &chassisUUID,
			Datapath:    "dp-1",
			ExternalIDs: map[string]string{
				"neutron:cidrs":        "192.168.1.10/24",
				"neutron:device_id":    "vm-uuid-1",
				"neutron:device_owner": "compute:az1",
				"neutron:host_id":      "host1",
			},
			MAC: []string{"fa:16:3e:aa:bb:cc 192.168.1.10"},
			Up:  &up,
		},
	}
	datapaths := []sb.DatapathBinding{
		{UUID: "dp-1", ExternalIDs: map[string]string{"logical-switch": "ls-1"}},
	}

	t.Run("excluded by default", func(t *testing.T) {
		resp := buildTopology(switches, nil, lsps, nil, chassisList, portBindings, datapaths, false)
		for _, n := range resp.Nodes {
			assert.NotEqual(t, "vm-port", n.Type)
		}
	})

	t.Run("included when enabled", func(t *testing.T) {
		resp := buildTopology(switches, nil, lsps, nil, chassisList, portBindings, datapaths, true)

		var vmNode TopologyNode
		for _, n := range resp.Nodes {
			if n.Type == "vm-port" {
				vmNode = n
			}
		}
		assert.Equal(t, "pb-vm-1", vmNode.ID)
		assert.Equal(t, "192.168.1.10/24", vmNode.Label)
		assert.Equal(t, "ch-1", vmNode.Group)
		assert.Equal(t, "vm-uuid-1", vmNode.Metadata["device_id"])
		assert.Equal(t, "compute:az1", vmNode.Metadata["device_owner"])
		assert.Equal(t, "fa:16:3e:aa:bb:cc 192.168.1.10", vmNode.Metadata["mac"])
		assert.Equal(t, "true", vmNode.Metadata["up"])

		// Should have vm-binding edge
		var hasVMEdge bool
		for _, e := range resp.Edges {
			if e.Type == "vm-binding" {
				hasVMEdge = true
				assert.Equal(t, "pb-vm-1", e.Source)
				assert.Equal(t, "ch-1", e.Target)
			}
		}
		assert.True(t, hasVMEdge, "expected vm-binding edge")
	})
}

func TestBuildTopology_VMPorts_SkipsNonVIF(t *testing.T) {
	chassisUUID := "ch-1"
	chassisList := []sb.Chassis{{UUID: "ch-1", Name: "host1"}}
	switches := []nb.LogicalSwitch{{UUID: "ls-1", Name: "s", Ports: []string{"lsp-1"}}}
	lsps := []nb.LogicalSwitchPort{{UUID: "lsp-1", Name: "p", Type: ""}}
	portBindings := []sb.PortBinding{
		{UUID: "pb-vif", LogicalPort: "p", Type: "", Chassis: &chassisUUID, Datapath: "dp-1"},
		{UUID: "pb-cr", LogicalPort: "cr-lrp1", Type: "chassisredirect", Chassis: &chassisUUID, Datapath: "dp-1"},
		{UUID: "pb-patch", LogicalPort: "patch1", Type: "patch", Chassis: &chassisUUID, Datapath: "dp-1"},
	}
	datapaths := []sb.DatapathBinding{{UUID: "dp-1", ExternalIDs: map[string]string{"logical-switch": "ls-1"}}}

	resp := buildTopology(switches, nil, lsps, nil, chassisList, portBindings, datapaths, true)

	vmCount := 0
	for _, n := range resp.Nodes {
		if n.Type == "vm-port" {
			vmCount++
		}
	}
	assert.Equal(t, 1, vmCount, "only VIF port (type='') should become vm-port node")
}

func TestEdgeKey_Deterministic(t *testing.T) {
	assert.Equal(t, edgeKey("a", "b"), edgeKey("b", "a"))
	assert.Equal(t, "a|b", edgeKey("a", "b"))
}
