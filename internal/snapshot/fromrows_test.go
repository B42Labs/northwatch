package snapshot_test

import (
	"context"
	"testing"
	"time"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildFromRows verifies that history-style rows (column values keyed by
// OVSDB tag, the form ovsdb.ModelToMap emits and the history store keeps) are
// reassembled into a snapshot that Serve can replay — in particular that
// multi-word columns like "external_ids" survive the tag→field remapping.
func TestBuildFromRows(t *testing.T) {
	const (
		lsUUID = "11111111-1111-4111-8111-111111111111"
		chUUID = "22222222-2222-4222-8222-222222222222"
	)

	rows := []snapshot.RowInput{
		{
			Database: "nb",
			Table:    "Logical_Switch",
			UUID:     lsUUID,
			Data: map[string]any{
				"_uuid":        lsUUID,
				"name":         "ls0",
				"external_ids": map[string]any{"owner": "team-net"},
				"other_config": map[string]any{},
				"ports":        []any{},
			},
		},
		{
			Database: "sb",
			Table:    "Chassis",
			UUID:     chUUID,
			Data: map[string]any{
				"_uuid":        chUUID,
				"name":         "ch0",
				"hostname":     "host0",
				"external_ids": map[string]any{"foo": "bar"},
				"other_config": map[string]any{},
			},
		},
		// An unknown table and an unknown database must be skipped, not fail.
		{Database: "nb", Table: "Does_Not_Exist", UUID: "x", Data: map[string]any{"name": "x"}},
		{Database: "elsewhere", Table: "Logical_Switch", UUID: "y", Data: map[string]any{"name": "y"}},
	}

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	file, err := snapshot.BuildFromRows(nbModel, sbModel, nb.Schema(), sb.Schema(), rows)
	require.NoError(t, err)
	require.Len(t, file.NB.Tables["Logical_Switch"], 1)
	require.Len(t, file.SB.Tables["Chassis"], 1)
	assert.NotContains(t, file.NB.Tables, "Does_Not_Exist")

	servers, err := snapshot.Serve(file, nbModel, sbModel, nb.Schema(), sb.Schema())
	require.NoError(t, err)
	defer servers.Close()

	ctx := context.Background()
	dbs, err := ovndb.Connect(ctx, servers.NBAddr, servers.SBAddr, nbModel, sbModel, ovndb.MonitorOptions{SkipServerMonitors: true})
	require.NoError(t, err)
	defer dbs.Close()

	var switches []nb.LogicalSwitch
	require.NoError(t, dbs.NB.List(ctx, &switches))
	require.Len(t, switches, 1)
	assert.Equal(t, "ls0", switches[0].Name)
	assert.Equal(t, lsUUID, switches[0].UUID, "original UUID must be preserved")
	// The multi-word column is the real test: a naive json round-trip would drop it.
	assert.Equal(t, map[string]string{"owner": "team-net"}, switches[0].ExternalIDs)

	var chassis []sb.Chassis
	require.NoError(t, dbs.SB.List(ctx, &chassis))
	require.Len(t, chassis, 1)
	assert.Equal(t, "ch0", chassis[0].Name)
	assert.Equal(t, "host0", chassis[0].Hostname)
	assert.Equal(t, map[string]string{"foo": "bar"}, chassis[0].ExternalIDs)
}

// TestBuildFromRows_PrunesDanglingRefs verifies that references to rows absent
// from the snapshot (because their table was not captured) are pruned, so the
// in-memory server's referential-integrity check passes instead of failing the
// whole load.
func TestBuildFromRows_PrunesDanglingRefs(t *testing.T) {
	const lsUUID = "33333333-3333-4333-8333-333333333333"
	rows := []snapshot.RowInput{
		{Database: "nb", Table: "Logical_Switch", UUID: lsUUID, Data: map[string]any{
			"_uuid":        lsUUID,
			"name":         "ls-refs",
			"ports":        []any{"aaaaaaaa-0000-4000-8000-000000000001"}, // absent LSP (set ref)
			"copp":         "bbbbbbbb-0000-4000-8000-000000000002",        // absent Copp (atomic ref)
			"external_ids": map[string]any{"k": "v"},
		}},
	}

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	file, err := snapshot.BuildFromRows(nbModel, sbModel, nb.Schema(), sb.Schema(), rows)
	require.NoError(t, err)

	servers, err := snapshot.Serve(file, nbModel, sbModel, nb.Schema(), sb.Schema())
	require.NoError(t, err)
	defer servers.Close()

	ctx := context.Background()
	dbs, err := ovndb.Connect(ctx, servers.NBAddr, servers.SBAddr, nbModel, sbModel, ovndb.MonitorOptions{SkipServerMonitors: true})
	require.NoError(t, err)
	defer dbs.Close()

	var switches []nb.LogicalSwitch
	require.NoError(t, dbs.NB.List(ctx, &switches))
	require.Len(t, switches, 1)
	assert.Equal(t, "ls-refs", switches[0].Name)
	assert.Empty(t, switches[0].Ports, "dangling port reference must be pruned")
	assert.Nil(t, switches[0].Copp, "dangling copp reference must be pruned")
	// Non-reference data is untouched.
	assert.Equal(t, map[string]string{"k": "v"}, switches[0].ExternalIDs)
}

// TestSuspendResumeMonitors exercises the live-connection suspend/resume cycle
// used while a snapshot is loaded: suspending empties the cache (without
// streaming), and resuming reloads every table. It guards against the cache
// inconsistency that arises if a cancelled monitor's rows are not purged before
// re-monitoring.
func TestSuspendResumeMonitors(t *testing.T) {
	const lsUUID = "44444444-4444-4444-8444-444444444444"
	rows := []snapshot.RowInput{
		{Database: "nb", Table: "Logical_Switch", UUID: lsUUID, Data: map[string]any{
			"_uuid": lsUUID, "name": "ls-live",
			"external_ids": map[string]any{}, "other_config": map[string]any{}, "ports": []any{},
		}},
	}

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	file, err := snapshot.BuildFromRows(nbModel, sbModel, nb.Schema(), sb.Schema(), rows)
	require.NoError(t, err)
	servers, err := snapshot.Serve(file, nbModel, sbModel, nb.Schema(), sb.Schema())
	require.NoError(t, err)
	defer servers.Close()

	ctx := context.Background()
	dbs, err := ovndb.Connect(ctx, servers.NBAddr, servers.SBAddr, nbModel, sbModel, ovndb.MonitorOptions{SkipServerMonitors: true})
	require.NoError(t, err)
	defer dbs.Close()

	count := func() int {
		var s []nb.LogicalSwitch
		require.NoError(t, dbs.NB.List(ctx, &s))
		return len(s)
	}
	require.Equal(t, 1, count())

	// Suspend: monitors cancelled and the cache purged. The libovsdb in-memory
	// test server does not implement monitor_cancel, so SuspendMonitors reports
	// that error here; the cache purge — the part that prevents the resume cache
	// inconsistency against a real server — still runs.
	if err := dbs.SuspendMonitors(ctx); err != nil {
		t.Logf("suspend reported (expected on in-memory server): %v", err)
	}
	assert.Equal(t, 0, count(), "cache must be emptied on suspend")

	// Resume re-monitors into the purged cache. Without the purge this would fail
	// with "cache inconsistent: cannot create row ... as it already exists" — the
	// exact failure this guards against.
	require.NoError(t, dbs.ResumeMonitors(ctx))
	require.Eventually(t, func() bool { return count() == 1 }, 2*time.Second, 10*time.Millisecond,
		"tables must be reloaded on resume")
}
