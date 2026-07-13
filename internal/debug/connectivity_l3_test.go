package debug

import (
	"context"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lrpModel builds a Logical_Router_Port model with all required map fields set.
func lrpModel(namedUUID, name string) *nb.LogicalRouterPort {
	return &nb.LogicalRouterPort{
		UUID:          namedUUID,
		Name:          name,
		MAC:           "00:00:00:00:00:aa",
		Networks:      []string{"10.0.1.1/24"},
		ExternalIDs:   map[string]string{},
		Options:       map[string]string{},
		Ipv6RaConfigs: map[string]string{},
		Status:        map[string]string{},
	}
}

// createSwitchWithRouterPort creates a Logical_Switch carrying one VIF LSP and
// one router-type LSP (pointing at lrpName via options:router-port) in a single
// transaction, returning the VIF port's real UUID.
func createSwitchWithRouterPort(t *testing.T, ctx context.Context, c client.Client, switchName, vifName, routerLSPName, lrpName string) string {
	t.Helper()
	vif := &nb.LogicalSwitchPort{UUID: "vif_" + vifName, Name: vifName, Addresses: []string{"00:00:00:00:00:01 10.0.1.10"}, ExternalIDs: map[string]string{}, Options: map[string]string{}}
	vifOps, err := c.Create(vif)
	require.NoError(t, err)
	rp := &nb.LogicalSwitchPort{UUID: "rp_" + routerLSPName, Name: routerLSPName, Type: "router", Options: map[string]string{"router-port": lrpName}, ExternalIDs: map[string]string{}}
	rpOps, err := c.Create(rp)
	require.NoError(t, err)
	ls := &nb.LogicalSwitch{Name: switchName, Ports: []string{"vif_" + vifName, "rp_" + routerLSPName}, ExternalIDs: map[string]string{}}
	lsOps, err := c.Create(ls)
	require.NoError(t, err)
	reply := transact(t, ctx, c, append(append(vifOps, rpOps...), lsOps...)...)
	vifUUID := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(ctx, &nb.LogicalSwitchPort{UUID: vifUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return vifUUID
}

// createSimpleSwitch creates a Logical_Switch with a single VIF LSP and returns
// the VIF port's real UUID.
func createSimpleSwitch(t *testing.T, ctx context.Context, c client.Client, switchName, vifName string) string {
	t.Helper()
	vif := &nb.LogicalSwitchPort{UUID: "vif_" + vifName, Name: vifName, Addresses: []string{"00:00:00:00:00:02 10.0.2.10"}, ExternalIDs: map[string]string{}, Options: map[string]string{}}
	vifOps, err := c.Create(vif)
	require.NoError(t, err)
	ls := &nb.LogicalSwitch{Name: switchName, Ports: []string{"vif_" + vifName}, ExternalIDs: map[string]string{}}
	lsOps, err := c.Create(ls)
	require.NoError(t, err)
	reply := transact(t, ctx, c, append(vifOps, lsOps...)...)
	vifUUID := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(ctx, &nb.LogicalSwitchPort{UUID: vifUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return vifUUID
}

// findRouterUUID returns the UUID of the Logical_Router with the given name.
func findRouterUUID(t *testing.T, ctx context.Context, c client.Client, name string) string {
	t.Helper()
	var routers []nb.LogicalRouter
	require.NoError(t, c.List(ctx, &routers))
	for _, r := range routers {
		if r.Name == name {
			return r.UUID
		}
	}
	t.Fatalf("router %q not found", name)
	return ""
}

func TestConnectivityChecker_CrossSwitchViaRouter(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	ctx := context.Background()
	checker := &ConnectivityChecker{NB: nbClient, SB: sbClient}

	// NB: a router with two ports, one facing each switch.
	lrpA := lrpModel("lrp_a", "lrp-a")
	lrpB := lrpModel("lrp_b", "lrp-b")
	lrpAOps, err := nbClient.Create(lrpA)
	require.NoError(t, err)
	lrpBOps, err := nbClient.Create(lrpB)
	require.NoError(t, err)
	lr := &nb.LogicalRouter{Name: "router-x", Ports: []string{"lrp_a", "lrp_b"}, ExternalIDs: map[string]string{}, Options: map[string]string{}}
	lrOps, err := nbClient.Create(lr)
	require.NoError(t, err)
	rtrOps := append(append(lrpAOps, lrpBOps...), lrOps...)
	transact(t, ctx, nbClient, rtrOps...)

	// NB: switch A with a VIF port and a router-type port referencing lrp-a.
	vifAUUID := createSwitchWithRouterPort(t, ctx, nbClient, "sw-a", "vm-a", "rp-a", "lrp-a")
	vifBUUID := createSwitchWithRouterPort(t, ctx, nbClient, "sw-b", "vm-b", "rp-b", "lrp-b")

	// SB: two chassis (so the encap-tunnel path runs) and up port bindings.
	// Use the counter-based testutil helpers throughout so no tunnel key
	// collides.
	chassis1UUID := testutil.InsertChassis(t, sbClient, "chassis-1", "host-1", "10.0.0.1")
	chassis2UUID := testutil.InsertChassis(t, sbClient, "chassis-2", "host-2", "10.0.0.2")
	up := true
	testutil.InsertPortBindingWithUp(t, sbClient, "vm-a", "", &chassis1UUID, &up)
	testutil.InsertPortBindingWithUp(t, sbClient, "vm-b", "", &chassis2UUID, &up)

	require.Eventually(t, func() bool {
		var pbs []sb.PortBinding
		return sbClient.List(ctx, &pbs) == nil && len(pbs) == 2
	}, 2*time.Second, 10*time.Millisecond)

	result, err := checker.Check(ctx, vifAUUID, vifBUUID)
	require.NoError(t, err)

	byName := map[string]ConnectivityCheck{}
	for _, ck := range result.Checks {
		byName[ck.Name] = ck
	}
	require.Contains(t, byName, "l2_connectivity")
	assert.Equal(t, StatusPass, byName["l2_connectivity"].Status)
	assert.Contains(t, byName["l2_connectivity"].Message, "router")
	require.Contains(t, byName, "l3_connectivity")
	assert.Equal(t, StatusPass, byName["l3_connectivity"].Status)
	require.Contains(t, byName, "encap_tunnel")
	assert.Equal(t, StatusPass, byName["encap_tunnel"].Status)
}

func TestConnectivityChecker_DifferentSwitchesNoRouter(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	ctx := context.Background()
	checker := &ConnectivityChecker{NB: nbClient, SB: sbClient}

	vifCUUID := createSimpleSwitch(t, ctx, nbClient, "sw-c", "vm-c")
	vifDUUID := createSimpleSwitch(t, ctx, nbClient, "sw-d", "vm-d")

	result, err := checker.Check(ctx, vifCUUID, vifDUUID)
	require.NoError(t, err)

	var l2 ConnectivityCheck
	for _, ck := range result.Checks {
		if ck.Name == "l2_connectivity" {
			l2 = ck
		}
	}
	assert.Equal(t, StatusFail, l2.Status, "no connecting router → L2 fail")
}

func TestConnectivityChecker_CheckL3Connectivity(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	ctx := context.Background()
	checker := &ConnectivityChecker{NB: nbClient}

	t.Run("RouterNotFound", func(t *testing.T) {
		got := checker.checkL3Connectivity(ctx, "does-not-exist")
		assert.Equal(t, StatusWarning, got.Status)
	})

	t.Run("RouterWithStaticRoutes", func(t *testing.T) {
		routerUUID := testutil.InsertRouterWithStaticRoutes(t, nbClient, "rtr-routes", "lrp-r", nil, nil,
			[]testutil.StaticRouteSpec{{IPPrefix: "0.0.0.0/0", Nexthop: "10.0.0.1"}})
		got := checker.checkL3Connectivity(ctx, routerUUID)
		assert.Equal(t, StatusPass, got.Status)
		assert.Contains(t, got.Message, "static routes")
	})

	t.Run("RouterWithNAT", func(t *testing.T) {
		testutil.InsertGatewayRouter(t, nbClient, "rtr-nat", "lrp-nat", nil, []string{"1.2.3.4"})
		routerUUID := findRouterUUID(t, ctx, nbClient, "rtr-nat")
		got := checker.checkL3Connectivity(ctx, routerUUID)
		assert.Equal(t, StatusPass, got.Status)
		assert.Contains(t, got.Message, "NAT rules")
	})
}

func TestConnectivityChecker_CheckACLs(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	ctx := context.Background()
	checker := &ConnectivityChecker{NB: nbClient}

	// A switch carrying one VIF port and one drop ACL matching that port.
	lsp := &nb.LogicalSwitchPort{UUID: "lsp_acl", Name: "vm-x", ExternalIDs: map[string]string{}, Options: map[string]string{}}
	lspOps, err := nbClient.Create(lsp)
	require.NoError(t, err)
	acl := &nb.ACL{UUID: "acl_deny", Action: "drop", Direction: "to-lport", Priority: 1000, Match: `outport == "vm-x"`, ExternalIDs: map[string]string{}, Options: map[string]string{}}
	aclOps, err := nbClient.Create(acl)
	require.NoError(t, err)
	ls := &nb.LogicalSwitch{Name: "sw-acl", Ports: []string{"lsp_acl"}, ACLs: []string{"acl_deny"}, ExternalIDs: map[string]string{}}
	lsOps, err := nbClient.Create(ls)
	require.NoError(t, err)
	reply := transact(t, ctx, nbClient, append(append(lspOps, aclOps...), lsOps...)...)
	lspUUID := reply[0].UUID.GoUUID
	switchUUID := reply[len(reply)-1].UUID.GoUUID

	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.LogicalSwitch{UUID: switchUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	resolved := &nb.LogicalSwitchPort{UUID: lspUUID}
	require.NoError(t, nbClient.Get(ctx, resolved))
	sw := &nb.LogicalSwitch{UUID: switchUUID}
	require.NoError(t, nbClient.Get(ctx, sw))

	t.Run("DropACLMatchingPortWarns", func(t *testing.T) {
		got := checker.checkACLs(ctx, resolved, resolved, sw, nil)
		assert.Equal(t, StatusWarning, got.Status)
		assert.Contains(t, got.Message, "drop/reject")
	})

	t.Run("NoACLsPasses", func(t *testing.T) {
		// A switch with no ACLs and no port groups → clean pass.
		empty := &nb.LogicalSwitch{ExternalIDs: map[string]string{}}
		got := checker.checkACLs(ctx, resolved, resolved, empty, nil)
		assert.Equal(t, StatusPass, got.Status)
		assert.Contains(t, got.Message, "No ACLs")
	})
}

func TestConnectivityChecker_CheckEncapTunnelMissingChassis(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)
	checker := &ConnectivityChecker{SB: sbClient}

	got := checker.checkEncapTunnel(context.Background(), "missing-src", "missing-dst")
	assert.Equal(t, StatusWarning, got.Status)
	assert.Contains(t, got.Message, "source chassis not found")
}
