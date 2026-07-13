package ovnsim

import (
	"os"
	"testing"

	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/testutil"
)

// TestMain quiets libovsdb's verbose stdr logging so test output stays readable
// and tears down the package's shared in-memory OVSDB server after the run.
func TestMain(m *testing.M) {
	stdr.SetVerbosity(0)
	os.Exit(testutil.Main(m))
}

// setupNB returns a connected, monitoring NB client backed by the package's
// shared in-memory NB server (one per package, wiped per test) instead of
// booting a dedicated server per call.
func setupNB(t *testing.T) client.Client {
	t.Helper()
	return testutil.SetupNBTestClient(t)
}
