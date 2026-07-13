package debug

import (
	"os"
	"testing"

	"github.com/b42labs/northwatch/internal/testutil"
)

// TestMain runs the package's tests against one shared in-memory OVSDB server
// (started lazily by testutil) and tears it down afterwards.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m))
}
