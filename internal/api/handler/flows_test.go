package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

