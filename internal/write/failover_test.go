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

	auditStore, err := NewAuditStore(historyStore.DB())
	require.NoError(t, err)

	registry := DefaultRegistry()
	return NewEngine(nbClient, nil, registry, collector, auditStore, 5*time.Minute, 0)
}

// haChassisSpec describes an HA_Chassis entry to create.
type haChassisSpec struct {
	chassisName string
	priority    int
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
		allOps = append(allOps, ovsdb.Operation{
			Op:    "insert",
			Table: "HA_Chassis",
			Row: map[string]interface{}{
				"chassis_name": s.chassisName,
				"priority":     s.priority,
			},
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
		{"chassis-1", 100},
		{"chassis-2", 75},
		{"chassis-3", 50},
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
		{"chassis-1", 100},
		{"chassis-2", 75},
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
		{"chassis-1", 100},
		{"chassis-2", 75},
	})
	// chassis-3 exists but is not in the group
	insertHAGroup(t, c, "other-group", []haChassisSpec{
		{"chassis-3", 50},
		{"chassis-4", 25},
	})

	engine := setupTestEngine(t, c)

	_, err := engine.Failover(context.Background(), "test-group", "chassis-3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in group")
}

func TestEvacuate(t *testing.T) {
	c := setupTestNBClient(t)

	// Group 1: chassis-1 is active
	_, g1UUIDs := insertHAGroup(t, c, "group-1", []haChassisSpec{
		{"chassis-1", 100},
		{"chassis-2", 75},
	})

	// Group 2: chassis-1 is also active
	_, g2UUIDs := insertHAGroup(t, c, "group-2", []haChassisSpec{
		{"chassis-1", 90},
		{"chassis-3", 60},
	})

	// Group 3: chassis-2 is active (should not be affected)
	_, g3UUIDs := insertHAGroup(t, c, "group-3", []haChassisSpec{
		{"chassis-2", 80},
		{"chassis-3", 40},
	})

	engine := setupTestEngine(t, c)

	plan, err := engine.Evacuate(context.Background(), "chassis-1")
	require.NoError(t, err)

	assert.Equal(t, "pending", plan.Status)
	// 2 groups affected, 2 ops each = 4 operations
	assert.Len(t, plan.Operations, 4)

	// Collect all affected UUIDs
	affectedUUIDs := make(map[string]bool)
	for _, op := range plan.Operations {
		affectedUUIDs[op.UUID] = true
	}

	// group-1 chassis should be affected
	assert.True(t, affectedUUIDs[g1UUIDs[0]])
	assert.True(t, affectedUUIDs[g1UUIDs[1]])
	// group-2 chassis should be affected
	assert.True(t, affectedUUIDs[g2UUIDs[0]])
	assert.True(t, affectedUUIDs[g2UUIDs[1]])
	// group-3 chassis should NOT be affected
	assert.False(t, affectedUUIDs[g3UUIDs[0]])
	assert.False(t, affectedUUIDs[g3UUIDs[1]])
}

func TestEvacuate_ChassisNotActive(t *testing.T) {
	c := setupTestNBClient(t)

	insertHAGroup(t, c, "test-group", []haChassisSpec{
		{"chassis-1", 100},
		{"chassis-2", 75},
	})

	engine := setupTestEngine(t, c)

	_, err := engine.Evacuate(context.Background(), "chassis-2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not the active chassis")
}
