package search

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyQuery(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "my-switch"},
		{UUID: "2", Name: "other"},
	})}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: tables}})
	results, truncated, err := engine.Search(context.Background(), "my-switch")
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, results, 1)
	assert.Equal(t, "nb", results[0].Database)
	assert.Len(t, results[0].Matches, 1)
	assert.Equal(t, "1", results[0].Matches[0]["_uuid"])
}

func TestSearch_MatchByExternalID(t *testing.T) {
	t.Parallel()
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "sw1", ExternalIDs: map[string]string{"neutron:network_id": "abc-123"}},
		{UUID: "2", Name: "sw2", ExternalIDs: map[string]string{"other": "val"}},
	})}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: tables}})
	results, truncated, err := engine.Search(context.Background(), "abc-123")
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Matches, 1)
}

func TestSearch_MatchByAddress(t *testing.T) {
	t.Parallel()
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "port1", Addresses: []string{"fa:16:3e:00:00:01 10.0.0.5"}},
		{UUID: "2", Name: "port2", Addresses: []string{"fa:16:3e:00:00:02 10.0.0.6"}},
	})}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: tables}})
	results, truncated, err := engine.Search(context.Background(), "10.0.0.5")
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Matches, 1)
	assert.Equal(t, "1", results[0].Matches[0]["_uuid"])
}

func TestSearch_CaseInsensitive(t *testing.T) {
	t.Parallel()
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "My-Switch"},
	})}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: tables}})
	results, truncated, err := engine.Search(context.Background(), "my-switch")
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, results, 1)
}

func TestSearch_EmptyQuery(t *testing.T) {
	t.Parallel()
	engine := NewEngine(nil)
	_, _, err := engine.Search(context.Background(), "")
	assert.Error(t, err)
}

func TestSearch_NoMatches(t *testing.T) {
	t.Parallel()
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "switch"},
	})}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: tables}})
	results, truncated, err := engine.Search(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Empty(t, results)
}

func TestSearch_CrossDB(t *testing.T) {
	t.Parallel()
	nbTables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "my-port"},
	})}
	sbTables := []TableDef{makeTestTable([]testRow{
		{UUID: "2", Name: "my-port-binding", ExternalIDs: map[string]string{"logical-port": "my-port"}},
	})}

	engine := NewEngine([]DatabaseTables{
		{Name: "nb", Tables: nbTables},
		{Name: "sb", Tables: sbTables},
	})
	results, truncated, err := engine.Search(context.Background(), "my-port")
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, results, 2)
	assert.Equal(t, "nb", results[0].Database)
	assert.Equal(t, "sb", results[1].Database)
}

func TestSearch_ArbitraryDatabaseName(t *testing.T) {
	t.Parallel()
	// The engine is no longer hardwired to "nb"/"sb": an arbitrary database name
	// (e.g. a per-chassis OVS instance) flows through verbatim to Result.Database.
	ovsTables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "br-int"},
	})}

	engine := NewEngine([]DatabaseTables{{Name: "ovs", Tables: ovsTables}})
	results, truncated, err := engine.Search(context.Background(), "br-int")
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, results, 1)
	assert.Equal(t, "ovs", results[0].Database)
	assert.Equal(t, "Test_Table", results[0].Table)
}

func TestSearch_MatchByMapKey(t *testing.T) {
	t.Parallel()
	tables := []TableDef{makeTestTable([]testRow{
		{UUID: "1", Name: "test", ExternalIDs: map[string]string{"neutron:network_id": "some-val"}},
	})}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: tables}})
	results, truncated, err := engine.Search(context.Background(), "neutron:network_id")
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, results, 1)
}

// TestSearch_PerTableCap covers the per-table bound: a query matching every row
// of a table returns at most maxMatchesPerTable of them and says so.
func TestSearch_PerTableCap(t *testing.T) {
	t.Parallel()
	rows := make([]testRow, maxMatchesPerTable+50)
	for i := range rows {
		rows[i] = testRow{UUID: fmt.Sprintf("uuid-%d", i), Name: fmt.Sprintf("switch-%d", i)}
	}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: []TableDef{makeTestTable(rows)}}})

	// "switch-" substring-matches every row, the pathological single-character
	// case the caps exist for.
	results, truncated, err := engine.Search(context.Background(), "switch-")
	require.NoError(t, err)
	assert.True(t, truncated)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Matches, maxMatchesPerTable)
}

// TestSearch_TotalCap covers the cross-table bound: enough tables each holding
// matches must not add up past maxTotalMatches, however many tables there are.
func TestSearch_TotalCap(t *testing.T) {
	t.Parallel()
	rows := make([]testRow, maxMatchesPerTable)
	for i := range rows {
		rows[i] = testRow{UUID: fmt.Sprintf("uuid-%d", i), Name: fmt.Sprintf("switch-%d", i)}
	}

	// maxTotalMatches/maxMatchesPerTable tables would exactly reach the total cap;
	// add three more so the loop must stop early.
	tableCount := maxTotalMatches/maxMatchesPerTable + 3
	tables := make([]TableDef, tableCount)
	for i := range tables {
		tables[i] = makeTestTable(rows)
	}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: tables}})

	results, truncated, err := engine.Search(context.Background(), "switch-")
	require.NoError(t, err)
	assert.True(t, truncated)

	total := 0
	for _, r := range results {
		total += len(r.Matches)
	}
	assert.Equal(t, maxTotalMatches, total)
	assert.Less(t, len(results), tableCount, "the scan must stop once the total cap is reached")
}

func TestSearch_ListErrorPropagates(t *testing.T) {
	t.Parallel()
	failing := TableDef{
		Name: "Broken_Table",
		ListFunc: func(context.Context) (any, error) {
			return nil, errors.New("cache read failed")
		},
	}

	engine := NewEngine([]DatabaseTables{{Name: "nb", Tables: []TableDef{failing}}})

	_, truncated, err := engine.Search(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "searching nb Broken_Table")
	assert.False(t, truncated)
}
