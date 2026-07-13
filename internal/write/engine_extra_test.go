package write

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/history"
	"github.com/b42labs/northwatch/internal/impact"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEngineWithClients builds an engine over the given NB and (optional) SB
// clients with an empty history collector and a fresh audit store. It mirrors
// setupTestEngine but allows an SB client to be supplied for SB-aware paths.
func setupEngineWithClients(t *testing.T, nbClient, sbClient client.Client) *Engine {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	historyStore, err := history.NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = historyStore.Close() })

	collector := history.NewCollector(historyStore, nil, nil, time.Hour, time.Hour)
	auditStore, err := NewAuditStore(context.Background(), historyStore.DB())
	require.NoError(t, err)

	engine, err := NewEngine(nbClient, sbClient, DefaultRegistry(), collector, auditStore, 5*time.Minute, 0)
	require.NoError(t, err)
	return engine
}

// insertLSWithPort creates a Logical_Switch that strongly references one
// Logical_Switch_Port (created in the same transaction so the non-root port is
// not garbage-collected). Returns the switch and port UUIDs.
func insertLSWithPort(t *testing.T, c client.Client, lsName, portName string) (string, string) {
	t.Helper()
	lspNamed := "lsp_" + portName
	lsp := &nb.LogicalSwitchPort{
		UUID:        lspNamed,
		Name:        portName,
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	lspOps, err := c.Create(lsp)
	require.NoError(t, err)

	ls := &nb.LogicalSwitch{
		Name:        lsName,
		Ports:       []string{lspNamed},
		ExternalIDs: map[string]string{},
	}
	lsOps, err := c.Create(ls)
	require.NoError(t, err)

	allOps := append(lspOps, lsOps...)
	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	lspUUID := reply[0].UUID.GoUUID
	lsUUID := reply[1].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LogicalSwitch{UUID: lsUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return lsUUID, lspUUID
}

// TestEngineSchema verifies Engine.Schema surfaces the registry's writable
// tables with per-column OVSDB types, exercising the schema-derivation path
// (Registry.Schema/tableSchema/goTypeToOVSDB).
func TestEngineSchema(t *testing.T) {
	engine := setupEngineWithClients(t, testutil.SetupNBTestClient(t), nil)

	schema := engine.Schema()
	require.NotEmpty(t, schema)

	byTable := make(map[string]TableSchema, len(schema))
	for _, ts := range schema {
		byTable[ts.Table] = ts
	}

	ls, ok := byTable["Logical_Switch"]
	require.True(t, ok, "Logical_Switch must be in the schema")
	assert.Equal(t, "nb", ls.Database)
	assert.False(t, ls.DeleteOnly)

	types := make(map[string]FieldInfo, len(ls.Fields))
	for _, f := range ls.Fields {
		types[f.Name] = f
	}
	// String, set<string>, and map<string,string> columns exercise the
	// scalar, slice, and map arms of goTypeToOVSDB.
	assert.Equal(t, "string", types["name"].Type)
	assert.Equal(t, "set<string>", types["ports"].Type)
	assert.Equal(t, "map<string,string>", types["external_ids"].Type)
	assert.True(t, types["_uuid"].ReadOnly, "_uuid must be read-only")

	// An integer column (ACL.priority) and a delete-only SB table.
	acl, ok := byTable["ACL"]
	require.True(t, ok)
	for _, f := range acl.Fields {
		if f.Name == "priority" {
			assert.Equal(t, "integer", f.Type)
		}
	}
	mb, ok := byTable["MAC_Binding"]
	require.True(t, ok)
	assert.True(t, mb.DeleteOnly, "MAC_Binding is delete-only")
	assert.Equal(t, "sb", mb.Database)
}

// TestEngineDeleteRoundTrip covers a delete operation end-to-end
// (preview→apply): computeDiffs/buildWaitOps/buildOVSDBOps delete arms and the
// success audit entry. It also asserts the deleted row is gone from NB.
func TestEngineDeleteRoundTrip(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)
	ctx := context.Background()

	uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-to-delete")

	plan, err := engine.Preview(ctx, []WriteOperation{{
		Action: "delete", Table: "Logical_Switch", UUID: uuid,
		Reason: "cleanup",
	}})
	require.NoError(t, err)
	require.Len(t, plan.Diffs, 1)
	assert.Equal(t, "delete", plan.Diffs[0].Action)
	assert.NotNil(t, plan.Diffs[0].Before, "delete diff must capture the pre-state")

	entry, err := engine.Apply(ctx, plan.ID, plan.ApplyToken, "tester")
	require.NoError(t, err)
	assert.Equal(t, "success", entry.Result)
	assert.Equal(t, "cleanup", entry.Reason)

	require.Eventually(t, func() bool {
		return nbClient.Get(ctx, &nb.LogicalSwitch{UUID: uuid}) != nil
	}, 2*time.Second, 10*time.Millisecond, "deleted switch must disappear from NB")
}

// TestEngineApplyErrorPaths covers Apply's rejection branches: an unknown plan,
// a wrong apply token, and a plan that fails re-validation at apply time
// because a referenced row was removed after preview.
func TestEngineApplyErrorPaths(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)
	ctx := context.Background()

	t.Run("unknown plan is rejected", func(t *testing.T) {
		_, err := engine.Apply(ctx, "no-such-plan", "token", "tester")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found or expired")
	})

	t.Run("wrong apply token is rejected", func(t *testing.T) {
		uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-badtoken")
		plan, err := engine.Preview(ctx, []WriteOperation{{
			Action: "update", Table: "Logical_Switch", UUID: uuid,
			Fields: jsonFields(t, `{"external_ids":{"k":"v"}}`),
		}})
		require.NoError(t, err)

		_, err = engine.Apply(ctx, plan.ID, "deadbeef", "tester")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid apply token")

		// The out-of-band value must be untouched (no mutation happened).
		ls := getLogicalSwitch(t, nbClient, uuid)
		assert.Empty(t, ls.ExternalIDs)
	})

	t.Run("apply-time revalidation failure records an error audit", func(t *testing.T) {
		lsUUID := testutil.InsertLogicalSwitch(t, nbClient, "ls-revalidate")
		lbUUID := seedLoadBalancer(t, nbClient, "lb-revalidate")

		plan, err := engine.Preview(ctx, []WriteOperation{{
			Action: "update", Table: "Logical_Switch", UUID: lsUUID,
			Fields: jsonFields(t, `{"load_balancer":["`+lbUUID+`"]}`),
		}})
		require.NoError(t, err)

		// Delete the referenced load balancer out of band; re-validation at
		// apply time must now reject the (previously valid) reference.
		deleteLoadBalancer(t, nbClient, lbUUID)

		entry, err := engine.Apply(ctx, plan.ID, plan.ApplyToken, "tester")
		require.Error(t, err)
		assert.True(t, IsInputError(err), "a vanished reference is a client error: %v", err)
		require.NotNil(t, entry)
		assert.Equal(t, "error", entry.Result)
		assert.Contains(t, entry.Error, "does not exist")
	})
}

// deleteLoadBalancer removes a Load_Balancer row by UUID and waits for the
// cache to reflect the deletion.
func deleteLoadBalancer(t *testing.T, c client.Client, uuid string) {
	t.Helper()
	lb := &nb.LoadBalancer{UUID: uuid}
	ops, err := c.Where(lb).Delete()
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LoadBalancer{UUID: uuid}) != nil
	}, 2*time.Second, 10*time.Millisecond)
}

// TestEngineAuditQueryAndCancel covers the audit read surface (QueryAudit,
// GetAuditEntry) against a persisted success entry, plus CancelPlan's
// found/not-found returns.
func TestEngineAuditQueryAndCancel(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)
	ctx := context.Background()

	t.Run("query and fetch a persisted audit entry", func(t *testing.T) {
		previewAndApply(t, engine, []WriteOperation{{
			Action: "create", Table: "Logical_Switch",
			Fields: jsonFields(t, `{"name":"ls-audit","external_ids":{"tier":"edge"}}`),
		}})

		entries, err := engine.QueryAudit(ctx, 10)
		require.NoError(t, err)
		require.NotEmpty(t, entries)
		assert.Equal(t, "success", entries[0].Result)

		got, err := engine.GetAuditEntry(ctx, entries[0].ID)
		require.NoError(t, err)
		assert.Equal(t, entries[0].ID, got.ID)
		require.Len(t, got.Operations, 1)
		assert.Equal(t, "create", got.Operations[0].Action)
	})

	t.Run("missing audit entry is an error", func(t *testing.T) {
		_, err := engine.GetAuditEntry(ctx, 999999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("cancel removes a pending plan then reports not-found", func(t *testing.T) {
		uuid := testutil.InsertLogicalSwitch(t, nbClient, "ls-cancel")
		plan, err := engine.Preview(ctx, []WriteOperation{{
			Action: "update", Table: "Logical_Switch", UUID: uuid,
			Fields: jsonFields(t, `{"external_ids":{"k":"v"}}`),
		}})
		require.NoError(t, err)

		assert.True(t, engine.CancelPlan(plan.ID), "first cancel removes the plan")
		_, ok := engine.GetPlan(plan.ID)
		assert.False(t, ok, "cancelled plan must not be retrievable")
		assert.False(t, engine.CancelPlan(plan.ID), "second cancel finds nothing")
	})
}

// TestEngineComputeImpactOnDelete verifies that when a resolver is configured, a
// delete preview carries impact analysis for the affected dependents.
func TestEngineComputeImpactOnDelete(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)
	engine.SetResolver(impact.NewResolver(nbClient, nil))
	ctx := context.Background()

	lsUUID, _ := insertLSWithPort(t, nbClient, "ls-impact", "port-impact")

	plan, err := engine.Preview(ctx, []WriteOperation{{
		Action: "delete", Table: "Logical_Switch", UUID: lsUUID,
	}})
	require.NoError(t, err)
	require.NotEmpty(t, plan.Impact, "deleting a switch with a port must report impact")
	assert.Equal(t, 0, plan.Impact[0].OperationIndex)
	assert.GreaterOrEqual(t, plan.Impact[0].Result.Summary.TotalAffected, 1)
}

// TestEngineStartStopsCleanup verifies the background cleanup goroutine starts
// and that the returned stop function returns promptly (the loop exits on
// context cancellation).
func TestEngineStartStopsCleanup(t *testing.T) {
	engine := setupEngineWithClients(t, testutil.SetupNBTestClient(t), nil)

	stop := engine.Start(context.Background())
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop() did not return; cleanup goroutine leaked")
	}
}

// TestEngineSBClientUnavailable verifies that touching an SB table without an
// SB client surfaces a clear error rather than panicking.
func TestEngineSBClientUnavailable(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)

	_, err := engine.Preview(context.Background(), []WriteOperation{{
		Action: "delete", Table: "MAC_Binding", UUID: "11111111-1111-1111-1111-111111111111",
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SB client not available")
}

// TestEngineBuildOVSDBOpsErrors covers buildOVSDBOps' error arms: an unknown
// action and an unregistered table.
func TestEngineBuildOVSDBOpsErrors(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)

	t.Run("unknown action", func(t *testing.T) {
		_, err := engine.buildOVSDBOps(nbClient, []WriteOperation{{
			Action: "frobnicate", Table: "Logical_Switch",
		}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown action")
	})

	t.Run("unregistered table", func(t *testing.T) {
		_, err := engine.buildOVSDBOps(nbClient, []WriteOperation{{
			Action: "create", Table: "Not_A_Table", Fields: map[string]any{"x": "y"},
		}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not writable")
	})
}
