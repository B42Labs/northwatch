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

	groups := buildFlowTableGroups(m)

	// Groups sorted by table_id ascending
	require.Len(t, groups, 2)
	assert.Equal(t, 0, groups[0].TableID)
	assert.Equal(t, 2, groups[1].TableID)

	// Flows sorted by priority descending
	assert.Equal(t, 100, groups[1].Flows[0].Priority)
	assert.Equal(t, 50, groups[1].Flows[1].Priority)
}

func TestBuildFlowTableGroups_Empty(t *testing.T) {
	groups := buildFlowTableGroups(nil)
	assert.Empty(t, groups)
}
