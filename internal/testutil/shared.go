package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/server"
)

// sharedServer is one lazily-started in-memory OVSDB server reused by every
// Setup*TestClient call in a package. Go compiles each package's tests into its
// own binary, so a package-level *sharedServer is a single server per package
// process — the "one shared OVSDB server per package" the test suite targets.
// Per-test isolation comes from wiping every table before each client monitors
// (see wipeAllTables), which is safe because OVSDB-backed tests run serially.
type sharedServer struct {
	once     sync.Once
	endpoint string // unix:<sock>
	sockDir  string
	srv      *server.OvsdbServer
	startErr error
}

var (
	sharedNB = &sharedServer{}
	sharedSB = &sharedServer{}

	// startedServers tracks the shared servers actually started this run so Main
	// can close them and remove their socket directories at the end.
	cleanupMu      sync.Mutex
	startedServers []*sharedServer
)

// endpointFor lazily starts the shared server for the given model/schema and
// returns its "unix:<sock>" endpoint. The first caller in the package starts
// the server; every later caller reuses it.
func (s *sharedServer) endpointFor(clientModel model.ClientDBModel, schema ovsdb.DatabaseSchema, sockName string) (string, error) {
	s.once.Do(func() {
		dbModel, errs := model.NewDatabaseModel(schema, clientModel)
		if len(errs) > 0 {
			s.startErr = fmt.Errorf("building %s db model: %v", schema.Name, errs)
			return
		}
		logger := stdr.New(nil)
		db := inmemory.NewDatabase(map[string]model.ClientDBModel{schema.Name: clientModel}, &logger)
		srv, err := server.NewOvsdbServer(db, &logger, dbModel)
		if err != nil {
			s.startErr = err
			return
		}
		endpoint, dir, err := serveShared(srv, sockName)
		if err != nil {
			s.startErr = err
			return
		}
		s.srv = srv
		s.sockDir = dir
		s.endpoint = endpoint

		cleanupMu.Lock()
		startedServers = append(startedServers, s)
		cleanupMu.Unlock()
	})
	return s.endpoint, s.startErr
}

// serveShared starts srv on a fresh unix socket and blocks until it is ready.
// Unlike serveUnixSocket it takes no *testing.T: a shared server outlives any
// single test, so its lifetime is owned by the package registry (torn down in
// Main), not by t.Cleanup. The socket directory uses os.MkdirTemp so the path
// stays under the macOS sun_path limit of 104 bytes.
func serveShared(srv *server.OvsdbServer, sockName string) (endpoint, dir string, err error) {
	dir, err = os.MkdirTemp("", "nw-shared")
	if err != nil {
		return "", "", err
	}
	sockPath := filepath.Join(dir, sockName)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve("unix", sockPath) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case serveErr := <-errCh:
			return "", dir, fmt.Errorf("serving %s on %s: %w", sockName, sockPath, serveErr)
		default:
		}
		if srv.Ready() {
			return "unix:" + sockPath, dir, nil
		}
		if time.Now().After(deadline) {
			return "", dir, fmt.Errorf("timed out waiting for %s server on %s to become ready", sockName, sockPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Main runs a package's tests against the lazily-started shared OVSDB servers
// and tears them down afterwards. Wire it up from a package's TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }
func Main(m *testing.M) int {
	code := m.Run()
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	for _, s := range startedServers {
		if s.srv != nil {
			s.srv.Close()
		}
		if s.sockDir != "" {
			_ = os.RemoveAll(s.sockDir)
		}
	}
	return code
}
