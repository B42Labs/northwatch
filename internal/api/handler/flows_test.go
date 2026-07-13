package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flowSeedTunnelKey hands out unique SB tunnel keys for rows seeded by the
// handler tests. It starts high so it never collides with the low keys that
// testutil's own Insert helpers use, letting a single test mix both.
var flowSeedTunnelKey atomic.Int64

func nextFlowSeedTunnelKey() int {
	return int(1_000_000 + flowSeedTunnelKey.Add(1))
}

// insertDatapath seeds a Datapath_Binding (a root SB table) carrying the given
// external_ids and returns its UUID.
func insertDatapath(t *testing.T, c client.Client, externalIDs map[string]string) string {
	t.Helper()
	if externalIDs == nil {
		externalIDs = map[string]string{}
	}
	dp := &sb.DatapathBinding{TunnelKey: nextFlowSeedTunnelKey(), ExternalIDs: externalIDs}
	ops, err := c.Create(dp)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.DatapathBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// insertLogicalFlow seeds a Logical_Flow (a root SB table) referencing the
// given datapath and returns its UUID.
func insertLogicalFlow(t *testing.T, c client.Client, dpUUID, pipeline string, tableID, priority int, match, actions string, externalIDs map[string]string) string {
	t.Helper()
	if externalIDs == nil {
		externalIDs = map[string]string{}
	}
	dp := dpUUID
	f := &sb.LogicalFlow{
		LogicalDatapath: &dp,
		Pipeline:        pipeline,
		TableID:         tableID,
		Priority:        priority,
		Match:           match,
		Actions:         actions,
		ExternalIDs:     externalIDs,
		Tags:            map[string]string{},
	}
	ops, err := c.Create(f)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.LogicalFlow{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// insertPortBindingOn seeds a Port_Binding on an existing datapath and returns
// its UUID.
func insertPortBindingOn(t *testing.T, c client.Client, logicalPort, dpUUID string) string {
	t.Helper()
	pb := &sb.PortBinding{
		LogicalPort: logicalPort,
		Datapath:    dpUUID,
		TunnelKey:   nextFlowSeedTunnelKey(),
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	ops, err := c.Create(pb)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.PortBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

func TestHandleFlows_HTTP(t *testing.T) {
	sbc := testutil.SetupSBTestClient(t)

	dpSwitch := insertDatapath(t, sbc, map[string]string{"logical-switch": "sw0"})
	insertLogicalFlow(t, sbc, dpSwitch, "ingress", 0, 100, `inport == "p0"`, "next;", map[string]string{"stage-name": "ls_in_port_sec"})
	insertLogicalFlow(t, sbc, dpSwitch, "ingress", 0, 50, "1", "drop;", nil)
	insertLogicalFlow(t, sbc, dpSwitch, "egress", 0, 50, "1", "output;", nil)

	dpRouter := insertDatapath(t, sbc, map[string]string{"logical-router": "lr0"})
	insertLogicalFlow(t, sbc, dpRouter, "ingress", 0, 100, "1", "next;", nil)

	mux := http.NewServeMux()
	RegisterFlows(mux, sbc)

	get := func(t *testing.T, url string) (*httptest.ResponseRecorder, FlowPipelineResponse) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var resp FlowPipelineResponse
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		}
		return w, resp
	}

	t.Run("switch datapath", func(t *testing.T) {
		w, resp := get(t, "/api/v1/flows?datapath="+dpSwitch)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "sw0", resp.DatapathName)
		require.Len(t, resp.Ingress, 1)
		assert.Equal(t, "ls_in_port_sec", resp.Ingress[0].TableName)
		require.Len(t, resp.Ingress[0].Flows, 2)
		// Flows are sorted by priority descending.
		assert.Equal(t, 100, resp.Ingress[0].Flows[0].Priority)
		require.Len(t, resp.Egress, 1)
	})

	t.Run("router datapath name", func(t *testing.T) {
		w, resp := get(t, "/api/v1/flows?datapath="+dpRouter)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "lr0", resp.DatapathName)
	})

	t.Run("missing datapath param", func(t *testing.T) {
		w, _ := get(t, "/api/v1/flows")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown datapath yields empty", func(t *testing.T) {
		w, resp := get(t, "/api/v1/flows?datapath=does-not-exist")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, resp.DatapathName)
		assert.Empty(t, resp.Ingress)
		assert.Empty(t, resp.Egress)
	})
}

func TestBuildFlowTableGroups_Sorting(t *testing.T) {
	m := map[int][]FlowEntry{
		2: {
			{UUID: "f1", Priority: 50, Match: "match1", Actions: "act1"},
			{UUID: "f2", Priority: 100, Match: "match2", Actions: "act2"},
		},
		0: {
			{UUID: "f3", Priority: 0, Match: "match3", Actions: "act3"},
		},
	}

	groups := buildFlowTableGroups(m, nil)

	// Groups sorted by table_id ascending
	require.Len(t, groups, 2)
	assert.Equal(t, 0, groups[0].TableID)
	assert.Equal(t, 2, groups[1].TableID)

	// Flows sorted by priority descending
	assert.Equal(t, 100, groups[1].Flows[0].Priority)
	assert.Equal(t, 50, groups[1].Flows[1].Priority)
}

func TestBuildFlowTableGroups_Empty(t *testing.T) {
	groups := buildFlowTableGroups(nil, nil)
	assert.Empty(t, groups)
}

func TestBuildFlowTableGroups_ExternalIDs(t *testing.T) {
	m := map[int][]FlowEntry{
		8: {
			{
				UUID:        "f1",
				Priority:    100,
				Match:       `inport == "port1"`,
				Actions:     "next;",
				ExternalIDs: map[string]string{"source": "acl-uuid-1", "stage-name": "ACL"},
			},
		},
	}

	groups := buildFlowTableGroups(m, map[int]string{8: "ACL"})

	require.Len(t, groups, 1)
	assert.Equal(t, "ACL", groups[0].TableName)
	require.Len(t, groups[0].Flows, 1)
	assert.Equal(t, map[string]string{"source": "acl-uuid-1", "stage-name": "ACL"}, groups[0].Flows[0].ExternalIDs)
}

func TestBuildFlowTableGroups_TableNameFallback(t *testing.T) {
	m := map[int][]FlowEntry{
		0: {{UUID: "f1", Priority: 0, Match: "1", Actions: "next;"}},
		7: {{UUID: "f2", Priority: 100, Match: "match", Actions: "next;"}},
	}

	// No stage-name from external_ids — should use static fallback
	groups := buildFlowTableGroups(m, nil)

	require.Len(t, groups, 2)
	assert.Equal(t, "Admission Control", groups[0].TableName)
	assert.Equal(t, "ACL Hints", groups[1].TableName)
}

func TestBuildFlowTableGroups_StageNameOverridesFallback(t *testing.T) {
	m := map[int][]FlowEntry{
		0: {{UUID: "f1", Priority: 0, Match: "1", Actions: "next;"}},
	}

	// Stage-name from external_ids should override static map
	groups := buildFlowTableGroups(m, map[int]string{0: "ls_in_check_port_sec"})

	require.Len(t, groups, 1)
	assert.Equal(t, "ls_in_check_port_sec", groups[0].TableName)
}
