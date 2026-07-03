package debug

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type aclSpec struct {
	priority int
	match    string
	action   string
}

// aclOps builds insert operations for the given ACL specs, tagged with named
// UUIDs derived from prefix (which must be a valid named-uuid, i.e. no hyphens).
// ACLs are GC'd if unreferenced, so callers must include the returned named
// UUIDs in the owning switch/port-group insert within the SAME transaction.
func aclOps(t *testing.T, c client.Client, prefix string, specs []aclSpec) ([]ovsdb.Operation, []string) {
	t.Helper()
	var ops []ovsdb.Operation
	named := make([]string, 0, len(specs))
	for i, s := range specs {
		u := fmt.Sprintf("%s_a%d", prefix, i)
		acl := &nb.ACL{UUID: u, Priority: s.priority, Direction: "from-lport", Match: s.match, Action: s.action}
		o, err := c.Create(acl)
		require.NoError(t, err)
		ops = append(ops, o...)
		named = append(named, u)
	}
	return ops, named
}

// seedSwitch creates a Logical_Switch and its ACLs in one transaction and waits
// for the client cache to observe the switch. name must be a valid named-uuid
// prefix (no hyphens).
func seedSwitch(t *testing.T, ctx context.Context, c client.Client, name string, specs []aclSpec) {
	t.Helper()
	ops, named := aclOps(t, c, name, specs)
	ls := &nb.LogicalSwitch{Name: name, ACLs: named}
	lsOps, err := c.Create(ls)
	require.NoError(t, err)
	ops = append(ops, lsOps...)
	transact(t, ctx, c, ops...)
	require.Eventually(t, func() bool {
		var got []nb.LogicalSwitch
		if err := c.List(ctx, &got); err != nil {
			return false
		}
		for _, s := range got {
			if s.Name == name {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

// seedPortGroup creates a Port_Group and its ACLs in one transaction.
func seedPortGroup(t *testing.T, ctx context.Context, c client.Client, name string, specs []aclSpec) {
	t.Helper()
	ops, named := aclOps(t, c, name, specs)
	pg := &nb.PortGroup{Name: name, ACLs: named}
	pgOps, err := c.Create(pg)
	require.NoError(t, err)
	ops = append(ops, pgOps...)
	transact(t, ctx, c, ops...)
	require.Eventually(t, func() bool {
		var got []nb.PortGroup
		if err := c.List(ctx, &got); err != nil {
			return false
		}
		for _, p := range got {
			if p.Name == name {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

func TestACLAudit_ScopedByAttachment(t *testing.T) {
	nbClient := setupNBClient(t)
	ctx := context.Background()

	// Two identical ACLs on two DIFFERENT switches: unrelated, must never be
	// reported as redundant against each other (the false-positive regression).
	seedSwitch(t, ctx, nbClient, "sw1", []aclSpec{{1000, `inport == "p1"`, "drop"}})
	seedSwitch(t, ctx, nbClient, "sw2", []aclSpec{{1000, `inport == "p1"`, "drop"}})

	// Same-switch shadow: same match, different action.
	seedSwitch(t, ctx, nbClient, "sw3", []aclSpec{
		{1000, `inport == "p9"`, "drop"},
		{900, `inport == "p9"`, "allow"},
	})

	result, err := (&ACLAuditor{NB: nbClient}).Audit(ctx)
	require.NoError(t, err)

	assert.Equal(t, 4, result.Total)
	// The cross-switch identical pair is never compared, so no redundant finding.
	assert.Equal(t, 0, result.Summary.Redundant, "identical ACLs on unrelated switches must not be reported")
	// The same-switch pair is still detected.
	assert.Equal(t, 1, result.Summary.Shadows)
	assert.False(t, result.Truncated)

	// The shadow finding carries the owning switch as context.
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "sw3", result.Findings[0].Context)
}

func TestACLAudit_PortGroupScope(t *testing.T) {
	nbClient := setupNBClient(t)
	ctx := context.Background()

	seedPortGroup(t, ctx, nbClient, "pg1", []aclSpec{
		{1000, "ip4", "drop"},
		{900, "ip4", "allow"},
	})

	result, err := (&ACLAuditor{NB: nbClient}).Audit(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Summary.Shadows, "port-group-scoped shadow must be detected")
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "pg1", result.Findings[0].Context)
}

func TestACLAudit_FindingsCapped(t *testing.T) {
	nbClient := setupNBClient(t)
	ctx := context.Background()

	// 35 identical ACLs on one switch => 35*34/2 = 595 redundant pairs, above the
	// maxACLFindings cap of 500.
	specs := make([]aclSpec, 0, 35)
	for i := 0; i < 35; i++ {
		specs = append(specs, aclSpec{100 + i, "ip4", "drop"})
	}
	seedSwitch(t, ctx, nbClient, "big", specs)

	result, err := (&ACLAuditor{NB: nbClient}).Audit(ctx)
	require.NoError(t, err)
	assert.True(t, result.Truncated)
	assert.Len(t, result.Findings, maxACLFindings)
}

func TestACLAudit_ContextCanceled(t *testing.T) {
	nbClient := setupNBClient(t)
	ctx := context.Background()

	seedSwitch(t, ctx, nbClient, "swx", []aclSpec{
		{1000, "ip4", "drop"},
		{900, "ip4", "allow"},
	})

	canceled, cancel := context.WithCancel(ctx)
	cancel()

	_, err := (&ACLAuditor{NB: nbClient}).Audit(canceled)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestMatchRelation(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected matchRelationType
	}{
		{"equal", `inport == "port1" && ip4.dst == 10.0.0.1`, `inport == "port1" && ip4.dst == 10.0.0.1`, matchEqual},
		{"superset - match-all", "1", `inport == "port1"`, matchSuperset},
		{"subset - match-all", `inport == "port1"`, "1", matchSubset},
		{"superset - fewer conjuncts", `inport == "port1"`, `inport == "port1" && ip4.dst == 10.0.0.1`, matchSuperset},
		{"subset - more conjuncts", `inport == "port1" && ip4.dst == 10.0.0.1`, `inport == "port1"`, matchSubset},
		{"disjoint", `inport == "port1"`, `inport == "port2"`, matchDisjoint},
		{"conflict", `inport == "port1" && ip4.dst == 10.0.0.1`, `inport == "port1" && ip4.dst == 10.0.0.2`, matchConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, matchRelation(tt.a, tt.b))
		})
	}
}

func TestParseConjuncts(t *testing.T) {
	result := parseConjuncts(`inport == "port1" && ip4.dst == 10.0.0.1 && tcp.dst == 80`)
	assert.Len(t, result, 3)
}

func TestCompareACLs_Shadow(t *testing.T) {
	higher := nb.ACL{UUID: "h", Priority: 1000, Direction: "from-lport", Match: `inport == "port1"`, Action: "drop"}
	lower := nb.ACL{UUID: "l", Priority: 900, Direction: "from-lport", Match: `inport == "port1" && ip4.dst == 10.0.0.1`, Action: "allow"}
	findings := compareACLs(higher, lower, map[string]string{})
	assert.Len(t, findings, 1)
	assert.Equal(t, "shadow", findings[0].Type)
}

func TestCompareACLs_Redundant(t *testing.T) {
	higher := nb.ACL{UUID: "h", Priority: 1000, Direction: "from-lport", Match: `inport == "port1"`, Action: "drop"}
	lower := nb.ACL{UUID: "l", Priority: 900, Direction: "from-lport", Match: `inport == "port1"`, Action: "drop"}
	findings := compareACLs(higher, lower, map[string]string{})
	assert.Len(t, findings, 1)
	assert.Equal(t, "redundant", findings[0].Type)
}

func TestCompareACLs_Conflict(t *testing.T) {
	higher := nb.ACL{UUID: "h", Priority: 1000, Direction: "from-lport", Match: `inport == "port1" && ip4.dst == 10.0.0.1`, Action: "drop"}
	lower := nb.ACL{UUID: "l", Priority: 900, Direction: "from-lport", Match: `inport == "port1" && ip4.dst == 10.0.0.2`, Action: "allow"}
	findings := compareACLs(higher, lower, map[string]string{})
	assert.Len(t, findings, 1)
	assert.Equal(t, "conflict", findings[0].Type)
}
