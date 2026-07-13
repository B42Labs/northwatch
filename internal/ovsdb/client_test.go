package ovsdb

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	libovsdb "github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupNBServer(t *testing.T) (string, func()) {
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

	return fmt.Sprintf("unix:%s", sockPath), func() {
		ovsdbServer.Close()
	}
}

func setupSBServer(t *testing.T) (string, func()) {
	t.Helper()

	clientModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)
	schema := sb.Schema()

	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)

	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{
		schema.Name: clientModel,
	}, &logger)

	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)

	sockPath := filepath.Join(t.TempDir(), "sb.sock")
	go func() {
		_ = ovsdbServer.Serve("unix", sockPath)
	}()

	require.Eventually(t, func() bool {
		return ovsdbServer.Ready()
	}, 5*time.Second, 10*time.Millisecond)

	return fmt.Sprintf("unix:%s", sockPath), func() {
		ovsdbServer.Close()
	}
}

func TestConnect(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{}, nil, nil)
	require.NoError(t, err)
	defer dbs.Close()

	assert.True(t, dbs.Ready())
}

func TestReady_WhileSuspended(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{}, nil, nil)
	require.NoError(t, err)
	defer dbs.Close()

	require.True(t, dbs.Ready())
	require.False(t, dbs.Suspended())

	// The in-memory test server does not implement MonitorCancel, so toggle the
	// suspended flag directly to exercise the Ready()/Suspended() gating.
	dbs.mu.Lock()
	dbs.suspended = true
	dbs.mu.Unlock()
	assert.True(t, dbs.Suspended())
	assert.False(t, dbs.Ready(), "Ready must be false while monitors are suspended")

	dbs.mu.Lock()
	dbs.suspended = false
	dbs.mu.Unlock()
	assert.False(t, dbs.Suspended())
	assert.True(t, dbs.Ready())
}

func TestMonitorOptions_ConnectTimeout(t *testing.T) {
	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)
	base := 30 * time.Second

	t.Run("no batching returns base", func(t *testing.T) {
		got := MonitorOptions{}.ConnectTimeout(base, nbModel, sbModel)
		assert.Equal(t, base, got)
	})

	t.Run("scales by table count and batch delay", func(t *testing.T) {
		nbCount := len(monitoredTables(nbModel, nil))
		sbCount := len(monitoredTables(sbModel, nil))
		maxCount := nbCount
		if sbCount > maxCount {
			maxCount = sbCount
		}
		require.Positive(t, maxCount)

		got := MonitorOptions{BatchDelay: time.Second}.ConnectTimeout(base, nbModel, sbModel)
		assert.Equal(t, base+time.Duration(maxCount)*time.Second, got)
	})

	t.Run("skip tables lower the count", func(t *testing.T) {
		all := MonitorOptions{BatchDelay: time.Second}.ConnectTimeout(base, nbModel, sbModel)
		skipped := MonitorOptions{BatchDelay: time.Second, SkipTables: monitoredTables(sbModel, nil)}.
			ConnectTimeout(base, nbModel, sbModel)
		// Skipping every SB table cannot make the timeout larger.
		assert.LessOrEqual(t, skipped, all)
	})
}

func TestConnect_InvalidNBAddr(t *testing.T) {
	nbAddr := fmt.Sprintf("unix:%s/nonexistent.sock", t.TempDir())

	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{}, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NB")
}

func TestConnect_InvalidSBAddr(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()

	sbAddr := fmt.Sprintf("unix:%s/nonexistent.sock", t.TempDir())

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{}, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SB")
}

func TestSplitEndpoints(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		count int
	}{
		{"single", "tcp:127.0.0.1:6641", 1},
		{"two", "tcp:10.0.0.1:6641,tcp:10.0.0.2:6641", 2},
		{"three with spaces", "tcp:10.0.0.1:6641, tcp:10.0.0.2:6641 , tcp:10.0.0.3:6641", 3},
		{"trailing comma", "tcp:127.0.0.1:6641,", 1},
		{"empty parts", "tcp:127.0.0.1:6641,,tcp:127.0.0.2:6641", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := splitEndpoints(tt.addr)
			assert.Len(t, opts, tt.count)
		})
	}
}

func TestConnect_CommaEndpoints(t *testing.T) {
	nbAddr1, nbCleanup1 := setupNBServer(t)
	defer nbCleanup1()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	// Use comma-separated endpoints (second is same as first for testing)
	nbAddr := nbAddr1 + "," + nbAddr1

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{}, nil, nil)
	require.NoError(t, err)
	defer dbs.Close()

	assert.True(t, dbs.Ready())
}

// seedNB writes the given models into a live NB OVSDB server using a throwaway
// writer client, so a later monitored Connect sees them as initial state.
func seedNB(t *testing.T, addr string, nbModel model.ClientDBModel, models ...model.Model) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w, err := client.NewOVSDBClient(nbModel, client.WithEndpoint(addr))
	require.NoError(t, err)
	require.NoError(t, w.Connect(ctx))
	defer w.Close()

	var ops []libovsdb.Operation
	for _, m := range models {
		o, err := w.Create(m)
		require.NoError(t, err)
		ops = append(ops, o...)
	}
	reply, err := w.Transact(ctx, ops...)
	require.NoError(t, err)
	_, err = libovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
}

func TestMonitoredTables(t *testing.T) {
	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)

	all := monitoredTables(nbModel, nil)
	require.NotEmpty(t, all)
	assert.True(t, sort.StringsAreSorted(all), "tables should be sorted")
	assert.Contains(t, all, "Logical_Switch")

	// A skipped table is removed; blank entries are ignored.
	filtered := monitoredTables(nbModel, []string{"Logical_Switch", "  ", ""})
	assert.NotContains(t, filtered, "Logical_Switch")
	assert.Len(t, filtered, len(all)-1)

	// Unknown table names are harmless no-ops.
	unknown := monitoredTables(nbModel, []string{"Does_Not_Exist"})
	assert.Len(t, unknown, len(all))
}

func TestConnect_StagedMonitoring(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	// Seed a row so we can confirm the staged per-table monitors still populate
	// the cache with each table's initial state.
	seedNB(t, nbAddr, nbModel, &nb.LogicalSwitch{Name: "staged-ls", ExternalIDs: map[string]string{}})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{BatchDelay: 2 * time.Millisecond}, nil, nil)
	require.NoError(t, err)
	defer dbs.Close()

	assert.True(t, dbs.Ready())
	require.Eventually(t, func() bool {
		var switches []nb.LogicalSwitch
		return dbs.NB.List(ctx, &switches) == nil && len(switches) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestConnect_SkipTables(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	// Seed one row in a table we will skip and one in a table we keep.
	seedNB(t, nbAddr, nbModel,
		&nb.LogicalSwitch{Name: "skip-me", ExternalIDs: map[string]string{}},
		&nb.LogicalRouter{Name: "keep-me", ExternalIDs: map[string]string{}, Options: map[string]string{}},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{SkipTables: []string{"Logical_Switch"}}, nil, nil)
	require.NoError(t, err)
	defer dbs.Close()

	// The kept table is populated...
	require.Eventually(t, func() bool {
		var routers []nb.LogicalRouter
		return dbs.NB.List(ctx, &routers) == nil && len(routers) == 1
	}, 2*time.Second, 10*time.Millisecond)

	// ...while the skipped table is never monitored, so it stays empty.
	var switches []nb.LogicalSwitch
	require.NoError(t, dbs.NB.List(ctx, &switches))
	assert.Empty(t, switches, "skipped table should not be populated")
}

func TestConnect_SkipAllTablesErrors(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	// Skipping every NB table leaves nothing to monitor on NB.
	var allNB []string
	for name := range nbModel.Types() {
		allNB = append(allNB, name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{SkipTables: allNB}, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tables left to monitor")
}

func TestReady_AfterClose(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{}, nil, nil)
	require.NoError(t, err)

	assert.True(t, dbs.Ready())
	dbs.Close()
	assert.False(t, dbs.Ready())
}
