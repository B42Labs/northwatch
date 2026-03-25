package impact

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transact is a test helper that runs a set of create operations in a single transaction.
func transact(t *testing.T, c client.Client, models ...model.Model) []string {
	t.Helper()
	ctx := context.Background()
	var allOps []ovsdb.Operation
	for _, m := range models {
		ops, err := c.Create(m)
		require.NoError(t, err)
		allOps = append(allOps, ops...)
	}
	reply, err := c.Transact(ctx, allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	uuids := make([]string, len(reply))
	for i, r := range reply {
		uuids[i] = r.UUID.GoUUID
	}
	return uuids
}

func TestResolve_SwitchPorts(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)
	ctx := context.Background()

	// Create 3 LSPs + 1 ACL + LogicalSwitch referencing them.
	namedLSP1 := "lsp_1"
	namedLSP2 := "lsp_2"
	namedLSP3 := "lsp_3"
	namedACL := "acl_1"

	uuids := transact(t, nbClient,
		&nb.LogicalSwitchPort{UUID: namedLSP1, Name: "port-1", ExternalIDs: map[string]string{}},
		&nb.LogicalSwitchPort{UUID: namedLSP2, Name: "port-2", ExternalIDs: map[string]string{}},
		&nb.LogicalSwitchPort{UUID: namedLSP3, Name: "port-3", ExternalIDs: map[string]string{}},
		&nb.ACL{UUID: namedACL, Action: "allow", Direction: "from-lport", Match: "1", ExternalIDs: map[string]string{}},
		&nb.LogicalSwitch{
			Name:        "test-switch",
			Ports:       []string{namedLSP1, namedLSP2, namedLSP3},
			ACLs:        []string{namedACL},
			ExternalIDs: map[string]string{},
		},
	)

	switchUUID := uuids[4]
	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.LogicalSwitch{UUID: switchUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	result, err := resolver.Resolve(ctx, "nb", "Logical_Switch", switchUUID)
	require.NoError(t, err)

	assert.Equal(t, "test-switch", result.Root.Name)
	assert.Equal(t, RefRoot, result.Root.RefType)

	// Should have 4 children: 3 ports (strong) + 1 ACL (strong).
	assert.Equal(t, 4, len(result.Root.Children))
	assert.Equal(t, 4, result.Summary.TotalAffected)
	assert.Equal(t, 3, result.Summary.ByTable["Logical_Switch_Port"])
	assert.Equal(t, 1, result.Summary.ByTable["ACL"])
}

func TestResolve_RouterFull(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)
	ctx := context.Background()

	namedLRP := "lrp_1"
	namedNAT := "nat_1"
	namedRoute := "route_1"

	uuids := transact(t, nbClient,
		&nb.LogicalRouterPort{UUID: namedLRP, Name: "rtr-port-1", MAC: "00:00:00:00:00:01", Networks: []string{"10.0.0.1/24"}},
		&nb.NAT{UUID: namedNAT, Type: "snat", ExternalIP: "1.2.3.4", LogicalIP: "10.0.0.0/24", ExternalIDs: map[string]string{}},
		&nb.LogicalRouterStaticRoute{UUID: namedRoute, IPPrefix: "0.0.0.0/0", Nexthop: "10.0.0.254", ExternalIDs: map[string]string{}},
		&nb.LogicalRouter{
			Name:         "test-router",
			Ports:        []string{namedLRP},
			Nat:          []string{namedNAT},
			StaticRoutes: []string{namedRoute},
			ExternalIDs:  map[string]string{},
		},
	)

	routerUUID := uuids[3]
	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.LogicalRouter{UUID: routerUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	result, err := resolver.Resolve(ctx, "nb", "Logical_Router", routerUUID)
	require.NoError(t, err)

	assert.Equal(t, "test-router", result.Root.Name)
	assert.Equal(t, 3, result.Summary.TotalAffected)
	assert.Equal(t, 1, result.Summary.ByTable["Logical_Router_Port"])
	assert.Equal(t, 1, result.Summary.ByTable["NAT"])
	assert.Equal(t, 1, result.Summary.ByTable["Logical_Router_Static_Route"])
}

func TestResolve_Recursive(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)
	ctx := context.Background()

	// Switch → Port → HA_Chassis_Group → HA_Chassis (3 levels deep).
	namedHAC := "hac_1"
	namedHACGroup := "hacg_1"
	namedLSP := "lsp_1"

	uuids := transact(t, nbClient,
		&nb.HAChassis{UUID: namedHAC, ChassisName: "chassis-1", Priority: 100, ExternalIDs: map[string]string{}},
		&nb.HAChassisGroup{UUID: namedHACGroup, Name: "ha-group-1", HaChassis: []string{namedHAC}, ExternalIDs: map[string]string{}},
		&nb.LogicalSwitchPort{UUID: namedLSP, Name: "ha-port", HaChassisGroup: strPtr(namedHACGroup), ExternalIDs: map[string]string{}},
		&nb.LogicalSwitch{Name: "ha-switch", Ports: []string{namedLSP}, ExternalIDs: map[string]string{}},
	)

	switchUUID := uuids[3]
	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.LogicalSwitch{UUID: switchUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	result, err := resolver.Resolve(ctx, "nb", "Logical_Switch", switchUUID)
	require.NoError(t, err)

	assert.Equal(t, 3, result.Summary.TotalAffected)
	assert.Equal(t, 3, result.Summary.MaxDepth)
	assert.Equal(t, 1, result.Summary.ByTable["Logical_Switch_Port"])
	assert.Equal(t, 1, result.Summary.ByTable["HA_Chassis_Group"])
	assert.Equal(t, 1, result.Summary.ByTable["HA_Chassis"])
}

func TestResolve_ReverseRefs(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)
	ctx := context.Background()

	// Create an LSP referenced by a PortGroup.
	namedLSP := "lsp_1"
	namedACL := "acl_pg"

	uuids := transact(t, nbClient,
		&nb.LogicalSwitchPort{UUID: namedLSP, Name: "pg-port", ExternalIDs: map[string]string{}},
		&nb.ACL{UUID: namedACL, Action: "drop", Direction: "to-lport", Match: "1", ExternalIDs: map[string]string{}},
		&nb.LogicalSwitch{Name: "pg-switch", Ports: []string{namedLSP}, ExternalIDs: map[string]string{}},
		&nb.PortGroup{Name: "test-pg", Ports: []string{namedLSP}, ACLs: []string{namedACL}, ExternalIDs: map[string]string{}},
	)

	lspUUID := uuids[0]
	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.PortGroup{UUID: uuids[3]}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	// Resolve the LSP — the PortGroup should appear as a reverse ref.
	result, err := resolver.Resolve(ctx, "nb", "Logical_Switch_Port", lspUUID)
	require.NoError(t, err)

	var foundReverse bool
	for _, child := range result.Root.Children {
		if child.Table == "Port_Group" && child.RefType == RefReverse {
			foundReverse = true
			assert.Equal(t, "test-pg", child.Name)
		}
	}
	assert.True(t, foundReverse, "expected Port_Group reverse reference")
}

func TestResolve_SBCorr(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	resolver := NewResolver(nbClient, sbClient)
	ctx := context.Background()

	// Create a LogicalSwitch in NB.
	switchUUID := testutil.InsertLogicalSwitch(t, nbClient, "correlated-switch")

	// Create a DatapathBinding in SB with external_ids pointing to the switch.
	dp := &sb.DatapathBinding{TunnelKey: 42, ExternalIDs: map[string]string{"logical-switch": switchUUID}}
	dpUUIDs := transact(t, sbClient, dp)
	dpUUID := dpUUIDs[0]

	require.Eventually(t, func() bool {
		return sbClient.Get(ctx, &sb.DatapathBinding{UUID: dpUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	// Create a PortBinding on that datapath.
	pb := &sb.PortBinding{
		LogicalPort: "correlated-port",
		Datapath:    dpUUID,
		TunnelKey:   1,
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	pbUUIDs := transact(t, sbClient, pb)
	require.Eventually(t, func() bool {
		return sbClient.Get(ctx, &sb.PortBinding{UUID: pbUUIDs[0]}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	result, err := resolver.Resolve(ctx, "nb", "Logical_Switch", switchUUID)
	require.NoError(t, err)

	// Should have at least a Datapath_Binding child with a Port_Binding grandchild.
	var dpNode *ImpactNode
	for i := range result.Root.Children {
		if result.Root.Children[i].Table == "Datapath_Binding" {
			dpNode = &result.Root.Children[i]
			break
		}
	}
	require.NotNil(t, dpNode, "expected Datapath_Binding correlation node")
	assert.Equal(t, RefCorrelation, dpNode.RefType)

	var pbNode *ImpactNode
	for i := range dpNode.Children {
		if dpNode.Children[i].Table == "Port_Binding" {
			pbNode = &dpNode.Children[i]
			break
		}
	}
	require.NotNil(t, pbNode, "expected Port_Binding under Datapath_Binding")
	assert.Equal(t, "correlated-port", pbNode.Name)
}

func TestResolve_NotFound(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)

	_, err := resolver.Resolve(context.Background(), "nb", "Logical_Switch", "00000000-0000-0000-0000-000000000000")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "entity not found")
}

func TestResolve_Leaf(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)
	ctx := context.Background()

	// Address_Set is a root table that can exist standalone and has no forward refs.
	uuids := transact(t, nbClient,
		&nb.AddressSet{Name: "leaf-set", Addresses: []string{"10.0.0.1"}, ExternalIDs: map[string]string{}},
	)
	asUUID := uuids[0]
	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.AddressSet{UUID: asUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	result, err := resolver.Resolve(ctx, "nb", "Address_Set", asUUID)
	require.NoError(t, err)

	assert.Empty(t, result.Root.Children)
	assert.Equal(t, 0, result.Summary.TotalAffected)
}

func TestResolve_BadDB(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)

	_, err := resolver.Resolve(context.Background(), "sb", "Port_Binding", "some-uuid")
	assert.ErrorIs(t, err, ErrUnsupportedDB)
}

func TestResolve_UnknownTable(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)

	_, err := resolver.Resolve(context.Background(), "nb", "Nonexistent_Table", "some-uuid")
	assert.ErrorIs(t, err, ErrUnknownTable)
}

func TestResolve_MaxDepthTruncation(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)
	resolver.SetLimits(1, 0) // depth=1, keep default maxEntities
	ctx := context.Background()

	// Switch → Port → HA_Chassis_Group → HA_Chassis — 3 levels, but maxDepth=1.
	namedHAC := "hac_1"
	namedHACGroup := "hacg_1"
	namedLSP := "lsp_1"

	uuids := transact(t, nbClient,
		&nb.HAChassis{UUID: namedHAC, ChassisName: "chassis-1", Priority: 100, ExternalIDs: map[string]string{}},
		&nb.HAChassisGroup{UUID: namedHACGroup, Name: "ha-group-1", HaChassis: []string{namedHAC}, ExternalIDs: map[string]string{}},
		&nb.LogicalSwitchPort{UUID: namedLSP, Name: "ha-port", HaChassisGroup: strPtr(namedHACGroup), ExternalIDs: map[string]string{}},
		&nb.LogicalSwitch{Name: "depth-switch", Ports: []string{namedLSP}, ExternalIDs: map[string]string{}},
	)

	switchUUID := uuids[3]
	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.LogicalSwitch{UUID: switchUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	result, err := resolver.Resolve(ctx, "nb", "Logical_Switch", switchUUID)
	require.NoError(t, err)

	// Depth=1 means only direct children (LSP). HA_Chassis_Group and HA_Chassis should be cut off.
	assert.True(t, result.Summary.Truncated, "expected truncation at depth limit")
	assert.Equal(t, 1, result.Summary.TotalAffected, "only the LSP should be included")
	assert.Equal(t, 1, result.Summary.MaxDepth)
}

func TestResolve_MaxEntitiesTruncation(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	resolver := NewResolver(nbClient, nil)
	// maxEntities=3 means: root (1 visited) + 2 children max before hitting the cap.
	resolver.SetLimits(0, 3)
	ctx := context.Background()

	namedLSP1 := "lsp_1"
	namedLSP2 := "lsp_2"
	namedLSP3 := "lsp_3"

	uuids := transact(t, nbClient,
		&nb.LogicalSwitchPort{UUID: namedLSP1, Name: "port-1", ExternalIDs: map[string]string{}},
		&nb.LogicalSwitchPort{UUID: namedLSP2, Name: "port-2", ExternalIDs: map[string]string{}},
		&nb.LogicalSwitchPort{UUID: namedLSP3, Name: "port-3", ExternalIDs: map[string]string{}},
		&nb.LogicalSwitch{
			Name:        "entity-switch",
			Ports:       []string{namedLSP1, namedLSP2, namedLSP3},
			ExternalIDs: map[string]string{},
		},
	)

	switchUUID := uuids[3]
	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.LogicalSwitch{UUID: switchUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	result, err := resolver.Resolve(ctx, "nb", "Logical_Switch", switchUUID)
	require.NoError(t, err)

	assert.True(t, result.Summary.Truncated, "expected truncation at entity limit")
	// Root counts as 1 visited, so only 2 of 3 ports should be included.
	assert.Equal(t, 2, result.Summary.TotalAffected)
}

func strPtr(s string) *string {
	return &s
}
