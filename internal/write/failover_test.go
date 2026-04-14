package write

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/history"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestNBClient(t *testing.T) client.Client {
	t.Helper()

	clientModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	schema := nb.Schema()

	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)

	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{
		schema.Name: clientModel,
	}, &logger)

	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)

	sockPath := filepath.Join(t.TempDir(), "nb.sock")
	go func() {
		_ = ovsdbServer.Serve("unix", sockPath)
	}()

	require.Eventually(t, func() bool {
		return ovsdbServer.Ready()
	}, 5*time.Second, 10*time.Millisecond)

	t.Cleanup(func() {
		ovsdbServer.Close()
	})

	c, err := client.NewOVSDBClient(
		clientModel,
		client.WithEndpoint(fmt.Sprintf("unix:%s", sockPath)),
	)
	require.NoError(t, err)

	err = c.Connect(context.Background())
	require.NoError(t, err)

	_, err = c.MonitorAll(context.Background())
	require.NoError(t, err)

	t.Cleanup(func() {
		c.Close()
	})

	return c
}

func setupTestEngine(t *testing.T, nbClient client.Client) *Engine {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	historyStore, err := history.NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = historyStore.Close() })

	collector := history.NewCollector(historyStore, nil, nil, time.Hour, time.Hour)

	auditStore, err := NewAuditStore(context.Background(), historyStore.DB())
	require.NoError(t, err)

	registry := DefaultRegistry()
	engine, err := NewEngine(nbClient, nil, registry, collector, auditStore, 5*time.Minute, 0)
	require.NoError(t, err)
	return engine
}

// haChassisSpec describes an HA_Chassis entry to create.
type haChassisSpec struct {
	chassisName string
	priority    int
	externalIDs map[string]string // optional
}

// insertHAGroup creates an HA_Chassis_Group with its HA_Chassis entries in a
// single transaction using raw OVSDB operations with named-uuid references so
// non-root HA_Chassis rows are not garbage collected.
// Returns the group UUID and chassis UUIDs (in the order of the specs).
func insertHAGroup(t *testing.T, c client.Client, groupName string, specs []haChassisSpec) (string, []string) {
	t.Helper()

	var allOps []ovsdb.Operation

	// Insert each HA_Chassis with a named UUID.
	for i, s := range specs {
		row := map[string]interface{}{
			"chassis_name": s.chassisName,
			"priority":     s.priority,
		}
		if len(s.externalIDs) > 0 {
			goMap := make(map[any]any, len(s.externalIDs))
			for k, v := range s.externalIDs {
				goMap[k] = v
			}
			row["external_ids"] = ovsdb.OvsMap{GoMap: goMap}
		}
		allOps = append(allOps, ovsdb.Operation{
			Op:       "insert",
			Table:    "HA_Chassis",
			Row:      row,
			UUIDName: fmt.Sprintf("hc-%d", i),
		})
	}

	// Build the ha_chassis set referencing the named UUIDs.
	haChassisSet := make([]any, len(specs))
	for i := range specs {
		haChassisSet[i] = ovsdb.UUID{GoUUID: fmt.Sprintf("hc-%d", i)}
	}
	allOps = append(allOps, ovsdb.Operation{
		Op:    "insert",
		Table: "HA_Chassis_Group",
		Row: map[string]interface{}{
			"name":       groupName,
			"ha_chassis": ovsdb.OvsSet{GoSet: haChassisSet},
		},
		UUIDName: "group",
	})

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	// Collect real UUIDs from reply.
	realChassisUUIDs := make([]string, len(specs))
	for i := range specs {
		realChassisUUIDs[i] = reply[i].UUID.GoUUID
	}
	groupUUID := reply[len(specs)].UUID.GoUUID

	// Wait for cache to reflect the group.
	require.Eventually(t, func() bool {
		g := &nb.HAChassisGroup{UUID: groupUUID}
		return c.Get(context.Background(), g) == nil
	}, 2*time.Second, 10*time.Millisecond)

	return groupUUID, realChassisUUIDs
}

func TestFailover(t *testing.T) {
	c := setupTestNBClient(t)

	_, chassisUUIDs := insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 75, nil},
		{"chassis-3", 50, nil},
	})
	hc1, _, hc3 := chassisUUIDs[0], chassisUUIDs[1], chassisUUIDs[2]

	engine := setupTestEngine(t, c)

	plan, err := engine.Failover(context.Background(), "test-group", "chassis-3")
	require.NoError(t, err)

	assert.Equal(t, "pending", plan.Status)
	assert.Len(t, plan.Operations, 2)

	// Verify priority swap: chassis-1 (100→50), chassis-3 (50→100)
	for _, op := range plan.Operations {
		assert.Equal(t, "update", op.Action)
		assert.Equal(t, "HA_Chassis", op.Table)
		switch op.UUID {
		case hc1:
			assert.Equal(t, 50, op.Fields["priority"])
		case hc3:
			assert.Equal(t, 100, op.Fields["priority"])
		default:
			t.Errorf("unexpected UUID in operation: %s", op.UUID)
		}
	}
}

func TestFailover_AlreadyLeader(t *testing.T) {
	c := setupTestNBClient(t)

	_, chassisUUIDs := insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 75, nil},
	})

	engine := setupTestEngine(t, c)

	// When the target already has the highest NB priority, Failover generates
	// a bump operation to trigger ovn-northd re-sync (NB/SB mismatch case).
	plan, err := engine.Failover(context.Background(), "test-group", "chassis-1")
	require.NoError(t, err)

	assert.Equal(t, "pending", plan.Status)
	assert.Len(t, plan.Operations, 1)
	assert.Equal(t, "update", plan.Operations[0].Action)
	assert.Equal(t, "HA_Chassis", plan.Operations[0].Table)
	assert.Equal(t, chassisUUIDs[0], plan.Operations[0].UUID)
	assert.Equal(t, 101, plan.Operations[0].Fields["priority"]) // bumped from 100
}

func TestFailover_GroupNotFound(t *testing.T) {
	c := setupTestNBClient(t)
	engine := setupTestEngine(t, c)

	_, err := engine.Failover(context.Background(), "nonexistent", "chassis-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFailover_ChassisNotInGroup(t *testing.T) {
	c := setupTestNBClient(t)

	insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 75, nil},
	})
	// chassis-3 exists but is not in the group
	insertHAGroup(t, c, "other-group", []haChassisSpec{
		{"chassis-3", 50, nil},
		{"chassis-4", 25, nil},
	})

	engine := setupTestEngine(t, c)

	_, err := engine.Failover(context.Background(), "test-group", "chassis-3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in group")
}

func TestEvacuate(t *testing.T) {
	c := setupTestNBClient(t)

	// Group 1: chassis-1 is active (highest priority)
	_, g1UUIDs := insertHAGroup(t, c, "group-1", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 75, nil},
	})

	// Group 2: chassis-1 is also active
	_, g2UUIDs := insertHAGroup(t, c, "group-2", []haChassisSpec{
		{"chassis-1", 90, nil},
		{"chassis-3", 60, nil},
	})

	// Group 3: chassis-2 is active (chassis-1 not a member — should not be affected)
	_, g3UUIDs := insertHAGroup(t, c, "group-3", []haChassisSpec{
		{"chassis-2", 80, nil},
		{"chassis-3", 40, nil},
	})

	engine := setupTestEngine(t, c)

	plan, err := engine.Evacuate(context.Background(), "chassis-1")
	require.NoError(t, err)

	assert.Equal(t, "pending", plan.Status)
	// Drain sets priority to 0 for each chassis-1 entry: 1 op per group = 2 ops
	assert.Len(t, plan.Operations, 2)

	// All ops should set priority to 0 and store original priority in external_ids
	affectedUUIDs := make(map[string]bool)
	for _, op := range plan.Operations {
		assert.Equal(t, "update", op.Action)
		assert.Equal(t, 0, op.Fields["priority"])
		extIDs, ok := op.Fields["external_ids"].(map[string]string)
		require.True(t, ok, "external_ids should be map[string]string")
		assert.Contains(t, extIDs, "northwatch:pre-drain-priority")
		affectedUUIDs[op.UUID] = true
	}

	// Only chassis-1 entries should be affected
	assert.True(t, affectedUUIDs[g1UUIDs[0]])  // chassis-1 in group-1
	assert.True(t, affectedUUIDs[g2UUIDs[0]])  // chassis-1 in group-2
	assert.False(t, affectedUUIDs[g1UUIDs[1]]) // chassis-2 in group-1 unaffected
	assert.False(t, affectedUUIDs[g2UUIDs[1]]) // chassis-3 in group-2 unaffected
	assert.False(t, affectedUUIDs[g3UUIDs[0]]) // group-3 unaffected
	assert.False(t, affectedUUIDs[g3UUIDs[1]])

	// Verify original priorities are preserved in external_ids
	opByUUID := make(map[string]WriteOperation)
	for _, op := range plan.Operations {
		opByUUID[op.UUID] = op
	}
	assert.Equal(t, "100", opByUUID[g1UUIDs[0]].Fields["external_ids"].(map[string]string)["northwatch:pre-drain-priority"])
	assert.Equal(t, "90", opByUUID[g2UUIDs[0]].Fields["external_ids"].(map[string]string)["northwatch:pre-drain-priority"])
}

func TestEvacuate_StandbyAlsoDrained(t *testing.T) {
	c := setupTestNBClient(t)

	// chassis-2 is not active but still gets drained (priority set to 0)
	_, chassisUUIDs := insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 75, nil},
	})

	engine := setupTestEngine(t, c)

	plan, err := engine.Evacuate(context.Background(), "chassis-2")
	require.NoError(t, err)

	assert.Len(t, plan.Operations, 1)
	assert.Equal(t, chassisUUIDs[1], plan.Operations[0].UUID)
	assert.Equal(t, 0, plan.Operations[0].Fields["priority"])
	extIDs := plan.Operations[0].Fields["external_ids"].(map[string]string)
	assert.Equal(t, "75", extIDs["northwatch:pre-drain-priority"])
}

func TestEvacuate_AlreadyDrained(t *testing.T) {
	c := setupTestNBClient(t)

	insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 0, nil}, // already drained
	})

	engine := setupTestEngine(t, c)

	_, err := engine.Evacuate(context.Background(), "chassis-2")
	assert.Error(t, err)
	assert.True(t, IsInputError(err))
	assert.Contains(t, err.Error(), "already drained")
}

func TestEvacuate_ChassisNotFound(t *testing.T) {
	c := setupTestNBClient(t)

	insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 75, nil},
	})

	engine := setupTestEngine(t, c)

	_, err := engine.Evacuate(context.Background(), "chassis-3")
	assert.Error(t, err)
	assert.True(t, IsInputError(err))
	assert.Contains(t, err.Error(), "not found")
}

func TestRestore(t *testing.T) {
	c := setupTestNBClient(t)

	// chassis-2 was previously drained (priority 0)
	_, g1UUIDs := insertHAGroup(t, c, "group-1", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 0, nil},
	})

	// chassis-2 also drained in another group
	_, g2UUIDs := insertHAGroup(t, c, "group-2", []haChassisSpec{
		{"chassis-2", 0, nil},
		{"chassis-3", 80, nil},
	})

	// Group where chassis-2 is not drained — should not be affected
	_, g3UUIDs := insertHAGroup(t, c, "group-3", []haChassisSpec{
		{"chassis-2", 50, nil},
		{"chassis-4", 30, nil},
	})

	engine := setupTestEngine(t, c)

	plan, err := engine.Restore(context.Background(), "chassis-2")
	require.NoError(t, err)

	assert.Equal(t, "pending", plan.Status)
	// Restore each drained entry: 2 groups affected
	assert.Len(t, plan.Operations, 2)

	affectedUUIDs := make(map[string]bool)
	for _, op := range plan.Operations {
		assert.Equal(t, "update", op.Action)
		// No external_ids marker → defaults to priority 1
		assert.Equal(t, 1, op.Fields["priority"])
		// Drain marker should be removed from external_ids
		extIDs, ok := op.Fields["external_ids"].(map[string]string)
		require.True(t, ok)
		assert.NotContains(t, extIDs, "northwatch:pre-drain-priority")
		affectedUUIDs[op.UUID] = true
	}

	assert.True(t, affectedUUIDs[g1UUIDs[1]])  // chassis-2 in group-1 (was 0)
	assert.True(t, affectedUUIDs[g2UUIDs[0]])  // chassis-2 in group-2 (was 0)
	assert.False(t, affectedUUIDs[g3UUIDs[0]]) // chassis-2 in group-3 (priority 50, not drained)
}

func TestRestore_NothingToDo(t *testing.T) {
	c := setupTestNBClient(t)

	insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 75, nil}, // not drained
	})

	engine := setupTestEngine(t, c)

	_, err := engine.Restore(context.Background(), "chassis-2")
	assert.Error(t, err)
	assert.True(t, IsInputError(err))
	assert.Contains(t, err.Error(), "no drained entries")
}

func TestRestore_OriginalPriority(t *testing.T) {
	c := setupTestNBClient(t)

	// Simulate a previously drained chassis that has the original priority saved
	_, g1UUIDs := insertHAGroup(t, c, "group-1", []haChassisSpec{
		{"chassis-1", 100, nil},
		{"chassis-2", 0, map[string]string{"northwatch:pre-drain-priority": "75"}},
	})

	_, g2UUIDs := insertHAGroup(t, c, "group-2", []haChassisSpec{
		{"chassis-2", 0, map[string]string{"northwatch:pre-drain-priority": "90"}},
		{"chassis-3", 80, nil},
	})

	engine := setupTestEngine(t, c)

	plan, err := engine.Restore(context.Background(), "chassis-2")
	require.NoError(t, err)

	assert.Len(t, plan.Operations, 2)

	opByUUID := make(map[string]WriteOperation)
	for _, op := range plan.Operations {
		opByUUID[op.UUID] = op
	}

	// Should restore to original priorities from external_ids
	assert.Equal(t, 75, opByUUID[g1UUIDs[1]].Fields["priority"])
	assert.Equal(t, 90, opByUUID[g2UUIDs[0]].Fields["priority"])

	// Drain marker should be removed
	for _, op := range plan.Operations {
		extIDs := op.Fields["external_ids"].(map[string]string)
		assert.NotContains(t, extIDs, "northwatch:pre-drain-priority")
	}
}
