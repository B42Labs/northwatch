package search

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected QueryType
	}{
		{"IPv4 address", "10.0.0.1", QueryIPv4},
		{"IPv4 with spaces", " 192.168.1.1 ", QueryIPv4},
		{"IPv6 address", "2001:db8::1", QueryIPv6},
		{"IPv6 full", "2001:0db8:0000:0000:0000:0000:0000:0001", QueryIPv6},
		{"MAC address colon", "aa:bb:cc:dd:ee:ff", QueryMAC},
		{"MAC address dash", "AA-BB-CC-DD-EE-FF", QueryMAC},
		{"UUID", "550e8400-e29b-41d4-a716-446655440000", QueryUUID},
		{"free text", "my-switch", QueryFreeText},
		{"partial IP (not valid)", "10.0.0", QueryFreeText},
		{"CIDR IPv4", "10.0.0.0/24", QueryIPv4},
		{"CIDR IPv6", "2001:db8::/32", QueryIPv6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ClassifyQuery(tt.query))
		})
	}
}

type testRow struct {
	UUID        string            `ovsdb:"_uuid"`
	Name        string            `ovsdb:"name"`
	ExternalIDs map[string]string `ovsdb:"external_ids"`
	Addresses   []string          `ovsdb:"addresses"`
}

func makeTestTable(rows []testRow) TableDef {
	return TableDef{
		Name: "Test_Table",
		ListFunc: func(ctx context.Context) (any, error) {
			return rows, nil
		},
	}
}

func TestSearch_MatchByName(t *testing.T) {
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "my-switch"},
		{UUID: "2", Name: "other"},
	})}

	engine := NewEngine(tables, nil)
	results, err := engine.Search(context.Background(), "my-switch")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "nb", results[0].Database)
	assert.Len(t, results[0].Matches, 1)
	assert.Equal(t, "1", results[0].Matches[0]["_uuid"])
}

func TestSearch_MatchByExternalID(t *testing.T) {
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "sw1", ExternalIDs: map[string]string{"neutron:network_id": "abc-123"}},
		{UUID: "2", Name: "sw2", ExternalIDs: map[string]string{"other": "val"}},
	})}

	engine := NewEngine(tables, nil)
	results, err := engine.Search(context.Background(), "abc-123")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Matches, 1)
}

func TestSearch_MatchByAddress(t *testing.T) {
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "port1", Addresses: []string{"fa:16:3e:00:00:01 10.0.0.5"}},
		{UUID: "2", Name: "port2", Addresses: []string{"fa:16:3e:00:00:02 10.0.0.6"}},
	})}

	engine := NewEngine(tables, nil)
	results, err := engine.Search(context.Background(), "10.0.0.5")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Matches, 1)
	assert.Equal(t, "1", results[0].Matches[0]["_uuid"])
}

func TestSearch_CaseInsensitive(t *testing.T) {
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "My-Switch"},
	})}

	engine := NewEngine(tables, nil)
	results, err := engine.Search(context.Background(), "my-switch")
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestSearch_EmptyQuery(t *testing.T) {
	engine := NewEngine(nil, nil)
	_, err := engine.Search(context.Background(), "")
	assert.Error(t, err)
}

func TestSearch_NoMatches(t *testing.T) {
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "switch"},
	})}

	engine := NewEngine(tables, nil)
	results, err := engine.Search(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_CrossDB(t *testing.T) {
	nbTables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "my-port"},
	})}
	sbTables := []TableDef{makeTestTable([]testRow{
		{UUID: "2", Name: "my-port-binding", ExternalIDs: map[string]string{"logical-port": "my-port"}},
	})}

	engine := NewEngine(nbTables, sbTables)
	results, err := engine.Search(context.Background(), "my-port")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "nb", results[0].Database)
	assert.Equal(t, "sb", results[1].Database)
}

func TestSearch_MatchByMapKey(t *testing.T) {
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "test", ExternalIDs: map[string]string{"neutron:network_id": "some-val"}},
	})}

	engine := NewEngine(tables, nil)
	results, err := engine.Search(context.Background(), "neutron:network_id")
	require.NoError(t, err)
	require.Len(t, results, 1)
}
