package write

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/history"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRollbackEngine builds an engine whose history collector captures
// Logical_Switch rows from nbClient, so snapshots have real NB data to roll
// back against.
func setupRollbackEngine(t *testing.T, nbClient client.Client) *Engine {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	historyStore, err := history.NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = historyStore.Close() })

	sources := []history.TableSource{{
		Database: "nb",
		Table:    "Logical_Switch",
		ListFunc: func(ctx context.Context) ([]map[string]any, error) {
			var switches []nb.LogicalSwitch
			if err := nbClient.List(ctx, &switches); err != nil {
				return nil, err
			}
			return api.ModelsToMaps(&switches), nil
		},
	}}
	collector := history.NewCollector(historyStore, nil, sources, time.Hour, time.Hour)

	auditStore, err := NewAuditStore(context.Background(), historyStore.DB())
	require.NoError(t, err)
	engine, err := NewEngine(nbClient, nil, DefaultRegistry(), collector, auditStore, 5*time.Minute, 0)
	require.NoError(t, err)
	return engine
}

// jsonFields builds an operation Fields map from a JSON literal so values arrive
// as map[string]any/[]any/float64/string, exactly as they would after HTTP decoding.
func jsonFields(t *testing.T, lit string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(lit), &m))
	return m
}

// previewAndApply runs an operation set through Preview then Apply and returns
// the resulting audit entry.
func previewAndApply(t *testing.T, engine *Engine, ops []WriteOperation) *AuditEntry {
	t.Helper()
	plan, err := engine.Preview(context.Background(), ops)
	require.NoError(t, err)
	entry, err := engine.Apply(context.Background(), plan.ID, plan.ApplyToken, "tester")
	require.NoError(t, err)
	return entry
}

// seedLoadBalancer inserts a Load_Balancer (a root table) directly and returns
// its server-assigned UUID, for reference-column round-trip tests.
func seedLoadBalancer(t *testing.T, c client.Client, name string) string {
	t.Helper()
	lb := &nb.LoadBalancer{Name: name}
	ops, err := c.Create(lb)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LoadBalancer{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

func getLogicalSwitch(t *testing.T, c client.Client, uuid string) nb.LogicalSwitch {
	t.Helper()
	ls := nb.LogicalSwitch{UUID: uuid}
	require.NoError(t, c.Get(context.Background(), &ls))
	return ls
}

// TestEngineApplyCreateWithMap covers the acceptance criterion: a write touching a
// map column round-trips through preview → apply → visible in NB. The pre-fix
// toOVSDBValue encoding marshaled external_ids as plain JSON and the apply failed.
func TestEngineApplyCreateWithMap(t *testing.T) {
	nbClient := setupTestNBClient(t)
	engine := setupTestEngine(t, nbClient)

	ops := []WriteOperation{{
		Action: "create",
		Table:  "Logical_Switch",
		Fields: jsonFields(t, `{"name":"ls-created","external_ids":{"owner":"northwatch","tier":"edge"}}`),
	}}
	previewAndApply(t, engine, ops)

	var switches []nb.LogicalSwitch
	require.NoError(t, nbClient.List(context.Background(), &switches))
	var found *nb.LogicalSwitch
	for i := range switches {
		if switches[i].Name == "ls-created" {
			found = &switches[i]
		}
	}
	require.NotNil(t, found, "created Logical_Switch not visible in NB")
	assert.Equal(t, map[string]string{"owner": "northwatch", "tier": "edge"}, found.ExternalIDs)
}

// TestEngineApplyUpdateNonScalar exercises update round-trips for map, set,
// reference-set, and optional-pointer columns. It shares one in-memory server
// across the subtests (distinct row names) to limit per-test server churn.
func TestEngineApplyUpdateNonScalar(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupTestEngine(t, nbClient)

	t.Run("map", func(t *testing.T) {
		uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-map")

		previewAndApply(t, engine, []WriteOperation{{
			Action: "update", Table: "Logical_Switch", UUID: uuid,
			Fields: jsonFields(t, `{"external_ids":{"key":"value"}}`),
		}})

		ls := getLogicalSwitch(t, nbClient, uuid)
		assert.Equal(t, map[string]string{"key": "value"}, ls.ExternalIDs)
	})

	t.Run("set", func(t *testing.T) {
		previewAndApply(t, engine, []WriteOperation{{
			Action: "create", Table: "Address_Set",
			Fields: jsonFields(t, `{"name":"as-set","addresses":["10.0.0.1","10.0.0.2"]}`),
		}})

		var sets []nb.AddressSet
		require.NoError(t, nbClient.List(context.Background(), &sets))
		var found *nb.AddressSet
		for i := range sets {
			if sets[i].Name == "as-set" {
				found = &sets[i]
			}
		}
		require.NotNil(t, found)
		assert.ElementsMatch(t, []string{"10.0.0.1", "10.0.0.2"}, found.Addresses)
	})

	t.Run("reference set", func(t *testing.T) {
		lsUUID := testutil.InsertLogicalSwitch(t, nbClient, "ls-ref")
		lbUUID := seedLoadBalancer(t, nbClient, "lb-ref")

		previewAndApply(t, engine, []WriteOperation{{
			Action: "update", Table: "Logical_Switch", UUID: lsUUID,
			Fields: jsonFields(t, `{"load_balancer":["`+lbUUID+`"]}`),
		}})

		ls := getLogicalSwitch(t, nbClient, lsUUID)
		assert.Equal(t, []string{lbUUID}, ls.LoadBalancer)
	})

	t.Run("optional pointer", func(t *testing.T) {
		previewAndApply(t, engine, []WriteOperation{{
			Action: "create", Table: "Logical_Router",
			Fields: jsonFields(t, `{"name":"lr-opt","enabled":true}`),
		}})

		var routers []nb.LogicalRouter
		require.NoError(t, nbClient.List(context.Background(), &routers))
		var found *nb.LogicalRouter
		for i := range routers {
			if routers[i].Name == "lr-opt" {
				found = &routers[i]
			}
		}
		require.NotNil(t, found)
		require.NotNil(t, found.Enabled)
		assert.True(t, *found.Enabled)
	})
}

func setLSExternalIDs(t *testing.T, c client.Client, uuid string, m map[string]string) {
	t.Helper()
	ls := &nb.LogicalSwitch{UUID: uuid}
	require.NoError(t, c.Get(context.Background(), ls))
	ls.ExternalIDs = m
	ops, err := c.Where(ls).Update(ls, &ls.ExternalIDs)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
}

func deleteLS(t *testing.T, c client.Client, uuid string) {
	t.Helper()
	ls := &nb.LogicalSwitch{UUID: uuid}
	ops, err := c.Where(ls).Delete()
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LogicalSwitch{UUID: uuid}) != nil
	}, 2*time.Second, 10*time.Millisecond)
}

// TestEngineApplyStaleState verifies the apply-time revalidation: an update
// whose target row changed since preview, or whose target was deleted, aborts
// with ErrStaleState and records an error audit entry.
func TestEngineApplyStaleState(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupTestEngine(t, nbClient)

	t.Run("updated row", func(t *testing.T) {
		uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-stale-upd")
		setLSExternalIDs(t, nbClient, uuid, map[string]string{"k": "v1"})

		plan, err := engine.Preview(context.Background(), []WriteOperation{{
			Action: "update", Table: "Logical_Switch", UUID: uuid,
			Fields: jsonFields(t, `{"external_ids":{"k":"v2"}}`),
		}})
		require.NoError(t, err)

		// Change the row out of band after preview.
		setLSExternalIDs(t, nbClient, uuid, map[string]string{"k": "other"})

		entry, err := engine.Apply(context.Background(), plan.ID, plan.ApplyToken, "tester")
		require.ErrorIs(t, err, ErrStaleState)
		require.NotNil(t, entry)
		assert.Equal(t, "error", entry.Result)

		// The out-of-band value must survive (no silent overwrite).
		ls := getLogicalSwitch(t, nbClient, uuid)
		assert.Equal(t, map[string]string{"k": "other"}, ls.ExternalIDs)
	})

	t.Run("deleted row", func(t *testing.T) {
		uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-stale-del")

		plan, err := engine.Preview(context.Background(), []WriteOperation{{
			Action: "update", Table: "Logical_Switch", UUID: uuid,
			Fields: jsonFields(t, `{"external_ids":{"k":"v"}}`),
		}})
		require.NoError(t, err)

		deleteLS(t, nbClient, uuid)

		_, err = engine.Apply(context.Background(), plan.ID, plan.ApplyToken, "tester")
		require.ErrorIs(t, err, ErrStaleState)
	})
}

// TestEngineRollbackRestoresFields verifies rollback restores a changed field on
// an existing row and reports (rather than recreates) rows deleted since the
// snapshot.
func TestEngineRollbackRestoresFields(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupRollbackEngine(t, nbClient)
	ctx := context.Background()

	t.Run("restores a changed field", func(t *testing.T) {
		uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-rollback")
		setLSExternalIDs(t, nbClient, uuid, map[string]string{"state": "original"})

		snap, err := engine.collector.TakeSnapshot(ctx, "test", "")
		require.NoError(t, err)

		// Diverge from the snapshot.
		setLSExternalIDs(t, nbClient, uuid, map[string]string{"state": "changed"})

		plan, err := engine.Rollback(ctx, snap.ID, "tester", "restore")
		require.NoError(t, err)
		require.Len(t, plan.Operations, 1, "exactly one field-restoring update expected")
		assert.Equal(t, "update", plan.Operations[0].Action)

		_, err = engine.Apply(ctx, plan.ID, plan.ApplyToken, "tester")
		require.NoError(t, err)

		ls := getLogicalSwitch(t, nbClient, uuid)
		assert.Equal(t, map[string]string{"state": "original"}, ls.ExternalIDs)
	})

	t.Run("reports deleted rows instead of recreating", func(t *testing.T) {
		uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-deleted")

		snap, err := engine.collector.TakeSnapshot(ctx, "test", "")
		require.NoError(t, err)

		deleteLS(t, nbClient, uuid)

		plan, err := engine.Rollback(ctx, snap.ID, "tester", "restore")
		require.NoError(t, err)
		require.NotEmpty(t, plan.Warnings)
		var mentioned bool
		for _, w := range plan.Warnings {
			if strings.Contains(w, "recreation is not supported") {
				mentioned = true
			}
		}
		assert.True(t, mentioned, "expected a non-recreation warning, got %v", plan.Warnings)
		for _, op := range plan.Operations {
			assert.NotEqual(t, "create", op.Action, "deleted rows must not be recreated")
		}
	})
}

// TestEngineDryRunNoPlanStored verifies DryRun computes diffs without persisting a
// plan or a snapshot.
func TestEngineDryRunNoPlanStored(t *testing.T) {
	nbClient := setupTestNBClient(t)
	engine := setupTestEngine(t, nbClient)

	plan, err := engine.DryRun(context.Background(), []WriteOperation{{
		Action: "create", Table: "Logical_Switch",
		Fields: jsonFields(t, `{"name":"ls-dry"}`),
	}})
	require.NoError(t, err)
	assert.Equal(t, "dry-run", plan.Status)
	require.Len(t, plan.Diffs, 1)

	_, ok := engine.GetPlan(plan.ID)
	assert.False(t, ok, "dry-run must not store a plan")
}
