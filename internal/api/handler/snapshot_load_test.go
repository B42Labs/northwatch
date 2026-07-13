package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/cluster"
	"github.com/b42labs/northwatch/internal/history"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/snapshotsession"
)

func TestSnapshotLoadEndpoints(t *testing.T) {
	ctx := context.Background()
	store, err := history.NewStore(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	const lsUUID = "11111111-1111-4111-8111-111111111111"
	meta, err := store.CreateSnapshot(ctx, "manual", "", []history.SnapshotRow{
		{Database: "nb", Table: "Logical_Switch", UUID: lsUUID, Data: map[string]any{
			"_uuid": lsUUID, "name": "ls0",
			"external_ids": map[string]any{}, "other_config": map[string]any{}, "ports": []any{},
		}},
	})
	require.NoError(t, err)

	nbModel, _ := nb.FullDatabaseModel()
	sbModel, _ := sb.FullDatabaseModel()
	reg := cluster.NewRegistry()
	mux := http.NewServeMux()
	proxy := RegisterClusterProxy(mux, reg, func(*http.ServeMux, *cluster.Cluster) {})

	mgr := snapshotsession.New(store, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
			m1, _ := nb.FullDatabaseModel()
			m2, _ := sb.FullDatabaseModel()
			dbs, cErr := ovndb.Connect(ctx, nbAddr, sbAddr, m1, m2, ovndb.MonitorOptions{SkipServerMonitors: true}, nil, nil)
			if cErr != nil {
				return nil, nil, cErr
			}
			return &cluster.Cluster{Name: name, Label: label, DBs: dbs}, nil, nil
		},
		func(c *cluster.Cluster) { proxy.Add(c.Name, http.NewServeMux()) },
		proxy.Remove,
		nil, // no live source in this HTTP-layer test
	)
	defer mgr.Close()
	RegisterSnapshotLoad(mux, mgr)

	do := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, method, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	// Invalid id → 400.
	assert.Equal(t, http.StatusBadRequest, do(http.MethodPost, "/api/v1/snapshots/abc/load").Code)

	// Unknown id → 404.
	assert.Equal(t, http.StatusNotFound, do(http.MethodPost, "/api/v1/snapshots/99999/load").Code)

	// Load the real snapshot → 200 with cluster metadata.
	idPath := "/api/v1/snapshots/" + strconv.FormatInt(meta.ID, 10)
	w := do(http.MethodPost, idPath+"/load")
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Cluster  string `json:"cluster"`
		Mode     string `json:"mode"`
		Snapshot struct {
			SourceID int64 `json:"sourceId"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "snapshot-"+strconv.FormatInt(meta.ID, 10), body.Cluster)
	assert.Equal(t, "snapshot", body.Mode)
	assert.Equal(t, meta.ID, body.Snapshot.SourceID)

	// Unload → 204.
	assert.Equal(t, http.StatusNoContent, do(http.MethodPost, idPath+"/unload").Code)

	// Unloading again → 404.
	assert.Equal(t, http.StatusNotFound, do(http.MethodPost, idPath+"/unload").Code)
}

func TestSnapshotLoad_TooManyReturns409(t *testing.T) {
	ctx := context.Background()
	store, err := history.NewStore(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	mkSnap := func(suffix string) int64 {
		uuid := ("11111111-1111-4111-8111-000000000000")[:36-len(suffix)] + suffix
		meta, cErr := store.CreateSnapshot(ctx, "manual", suffix, []history.SnapshotRow{
			{Database: "nb", Table: "Logical_Switch", UUID: uuid, Data: map[string]any{
				"_uuid": uuid, "name": "ls-" + suffix,
				"external_ids": map[string]any{}, "other_config": map[string]any{}, "ports": []any{},
			}},
		})
		require.NoError(t, cErr)
		return meta.ID
	}
	id1, id2 := mkSnap("aaaa"), mkSnap("bbbb")

	nbModel, _ := nb.FullDatabaseModel()
	sbModel, _ := sb.FullDatabaseModel()
	reg := cluster.NewRegistry()
	mux := http.NewServeMux()
	proxy := RegisterClusterProxy(mux, reg, func(*http.ServeMux, *cluster.Cluster) {})
	mgr := snapshotsession.New(store, reg, nbModel, sbModel, nb.Schema(), sb.Schema(),
		func(name, label, nbAddr, sbAddr string) (*cluster.Cluster, []func(), error) {
			m1, _ := nb.FullDatabaseModel()
			m2, _ := sb.FullDatabaseModel()
			dbs, cErr := ovndb.Connect(ctx, nbAddr, sbAddr, m1, m2, ovndb.MonitorOptions{SkipServerMonitors: true}, nil, nil)
			if cErr != nil {
				return nil, nil, cErr
			}
			return &cluster.Cluster{Name: name, Label: label, DBs: dbs}, nil, nil
		},
		func(c *cluster.Cluster) { proxy.Add(c.Name, http.NewServeMux()) },
		proxy.Remove,
		nil,
	)
	defer mgr.Close()
	mgr.SetMaxLoaded(1)
	RegisterSnapshotLoad(mux, mgr)

	do := func(id int64) int {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/snapshots/"+strconv.FormatInt(id, 10)+"/load", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}

	require.Equal(t, http.StatusOK, do(id1))
	assert.Equal(t, http.StatusConflict, do(id2), "second load beyond the cap must be 409")
}
