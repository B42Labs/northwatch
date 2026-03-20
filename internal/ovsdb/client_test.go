package ovsdb

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
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

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel)
	require.NoError(t, err)
	defer dbs.Close()

	assert.True(t, dbs.Ready())
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

	_, err = Connect(ctx, nbAddr, sbAddr, nbModel, sbModel)
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

	_, err = Connect(ctx, nbAddr, sbAddr, nbModel, sbModel)
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

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel)
	require.NoError(t, err)
	defer dbs.Close()

	assert.True(t, dbs.Ready())
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

	dbs, err := Connect(ctx, nbAddr, sbAddr, nbModel, sbModel)
	require.NoError(t, err)

	assert.True(t, dbs.Ready())
	dbs.Close()
	assert.False(t, dbs.Ready())
}
