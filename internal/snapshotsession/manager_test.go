package snapshotsession_test

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/b42labs/northwatch/internal/cluster"
	"github.com/b42labs/northwatch/internal/history"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/snapshotsession"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *history.Store {
	t.Helper()
	store, err := history.NewStore(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestManagerLoadUnload drives a stored history snapshot through the full
// runtime path: export → reassemble → serve → register as a read-only cluster,
// then verify the data is queryable through that cluster and that Unload tears
// everything down.
func TestManagerLoadUnload(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	const lsUUID = "11111111-1111-4111-8111-111111111111"
	rows := []history.SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: lsUUID, Data: map[string]any{
			"_uuid":        lsUUID,
			"name":         "ls0",
			"external_ids": map[string]any{"owner": "team-net"},
			"other_config": map[string]any{},
			"ports":        []any{},
		}},
	}
	meta, err := store.CreateSnapshot(ctx, "manual", "test", rows)
	require.NoError(t, err)

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	reg := cluster.NewRegistry()
	registered := map[string]bool{}

	build := func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
		m1, _ := nb.FullDatabaseModel()
		m2, _ := sb.FullDatabaseModel()
		dbs, err := ovndb.Connect(ctx, nbAddr, sbAddr, m1, m2, ovndb.MonitorOptions{SkipServerMonitors: true})
		if err != nil {
			return nil, nil, err
		}
		return &cluster.Cluster{Name: name, Label: label, DBs: dbs}, nil, nil
	}

	live := &fakeLive{}

	mgr := snapshotsession.New(store, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		build,
		func(c *cluster.Cluster) { registered[c.Name] = true },
		func(name string) { delete(registered, name) },
		live,
	)
	defer mgr.Close()

	want := "snapshot-" + strconv.FormatInt(meta.ID, 10)

	ld, err := mgr.Load(ctx, meta.ID)
	require.NoError(t, err)
	assert.Equal(t, want, ld.Cluster)
	assert.Equal(t, "snapshot", ld.Mode)
	assert.Equal(t, meta.ID, ld.SourceID)
	assert.True(t, registered[want], "routes must be registered")
	assert.Equal(t, 1, live.suspends, "live OVN must be suspended on first load")
	assert.Equal(t, 0, live.resumes)

	c, ok := reg.Get(want)
	require.True(t, ok, "cluster must be in the registry")
	assert.Equal(t, "snapshot", c.Mode)
	require.NotNil(t, c.Snapshot)
	assert.Equal(t, meta.ID, c.Snapshot.SourceID)

	// The snapshot data is queryable through the loaded cluster's client.
	var switches []nb.LogicalSwitch
	require.NoError(t, c.DBs.NB.List(ctx, &switches))
	require.Len(t, switches, 1)
	assert.Equal(t, "ls0", switches[0].Name)
	assert.Equal(t, map[string]string{"owner": "team-net"}, switches[0].ExternalIDs)

	// Loading the same snapshot again is idempotent and does not re-suspend.
	ld2, err := mgr.Load(ctx, meta.ID)
	require.NoError(t, err)
	assert.Equal(t, ld.Cluster, ld2.Cluster)
	assert.Len(t, reg.List(), 1, "no duplicate cluster")
	assert.Equal(t, 1, live.suspends, "idempotent load must not suspend again")

	// Unload tears down registry entry and routing, and resumes live OVN.
	require.NoError(t, mgr.Unload(ctx, meta.ID))
	_, ok = reg.Get(want)
	assert.False(t, ok)
	assert.False(t, registered[want])
	assert.Equal(t, 1, live.resumes, "live OVN must be resumed when the last snapshot is ejected")

	// Unloading again reports not-loaded.
	assert.ErrorIs(t, mgr.Unload(ctx, meta.ID), snapshotsession.ErrNotLoaded)

	// Loading an unknown snapshot surfaces history.ErrNotFound.
	_, err = mgr.Load(ctx, 99999)
	assert.ErrorIs(t, err, history.ErrNotFound)
}

// fakeLive records SuspendMonitors/ResumeMonitors calls.
type fakeLive struct {
	suspends int
	resumes  int
}

func (f *fakeLive) SuspendMonitors(context.Context) error {
	f.suspends++
	return nil
}

func (f *fakeLive) ResumeMonitors(context.Context) error {
	f.resumes++
	return nil
}
