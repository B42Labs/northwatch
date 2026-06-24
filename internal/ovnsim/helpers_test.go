package ovnsim

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/server"
	"github.com/stretchr/testify/require"
)

// TestMain quiets libovsdb's verbose stdr logging so test output stays readable.
func TestMain(m *testing.M) {
	stdr.SetVerbosity(0)
	os.Exit(m.Run())
}

// setupNB starts an in-memory NB OVSDB server and returns a connected,
// monitoring client.
//
// The socket lives under a short /tmp directory rather than t.TempDir(): a unix
// socket path must fit in sockaddr_un.sun_path (104 bytes on macOS), and
// t.TempDir() embeds the (long) test name, so longer-named tests would
// intermittently exceed the limit and net.Listen would fail — surfacing only as
// a connection timeout. Keeping the path short makes the suite reliable.
func setupNB(t *testing.T) client.Client {
	t.Helper()

	clientModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	schema := nb.Schema()
	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)

	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{schema.Name: clientModel}, &logger)
	srv, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)

	dir, err := os.MkdirTemp("/tmp", "nws")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")

	go func() { _ = srv.Serve("unix", sock) }()
	t.Cleanup(srv.Close)
	require.Eventually(t, srv.Ready, 10*time.Second, 10*time.Millisecond, "in-memory NB server never became ready")

	c, err := client.NewOVSDBClient(clientModel, client.WithEndpoint("unix:"+sock))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c.Connect(ctx))
	_, err = c.MonitorAll(ctx)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c
}
