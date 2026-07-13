package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/debug"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertSwitchWithPort seeds a Logical_Switch and one Logical_Switch_Port (a
// non-root table, so both are created in one transaction) and returns their
// UUIDs.
func insertSwitchWithPort(t *testing.T, c client.Client, switchName, portName string) (string, string) {
	t.Helper()
	namedLSP := "lsp_" + portName
	lsp := &nb.LogicalSwitchPort{UUID: namedLSP, Name: portName, ExternalIDs: map[string]string{}}
	lspOps, err := c.Create(lsp)
	require.NoError(t, err)
	ls := &nb.LogicalSwitch{Name: switchName, Ports: []string{namedLSP}, ExternalIDs: map[string]string{}}
	lsOps, err := c.Create(ls)
	require.NoError(t, err)
	ops := append(lspOps, lsOps...)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	lspUUID := reply[0].UUID.GoUUID
	switchUUID := reply[1].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LogicalSwitchPort{UUID: lspUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return switchUUID, lspUUID
}

func TestRegisterDebug_HTTP(t *testing.T) {
	nbc := testutil.SetupNBTestClient(t)
	sbc := testutil.SetupSBTestClient(t)

	_, lspUUID := insertSwitchWithPort(t, nbc, "sw-dbg", "port-dbg")
	_, lspUUID2 := insertSwitchWithPort(t, nbc, "sw-dbg2", "port-dbg2")

	checker := &debug.ConnectivityChecker{NB: nbc, SB: sbc}
	diagnoser := &debug.PortDiagnoser{NB: nbc, SB: sbc}
	auditor := &debug.ACLAuditor{NB: nbc}
	detector := &debug.StaleDetector{NB: nbc, SB: sbc}

	mux := http.NewServeMux()
	RegisterDebug(mux, checker, diagnoser, auditor, detector)

	do := func(t *testing.T, url string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	t.Run("port diagnostics all", func(t *testing.T) {
		w := do(t, "/api/v1/debug/port-diagnostics")
		require.Equal(t, http.StatusOK, w.Code)
		var summary debug.PortDiagnosticsSummary
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
		assert.Equal(t, 2, summary.Total)
		assert.Len(t, summary.Ports, 2)
	})

	t.Run("port diagnostics limit caps results", func(t *testing.T) {
		w := do(t, "/api/v1/debug/port-diagnostics?limit=1")
		require.Equal(t, http.StatusOK, w.Code)
		var summary debug.PortDiagnosticsSummary
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
		// Counts always reflect the full diagnosis; only Ports is capped.
		assert.Equal(t, 2, summary.Total)
		assert.Len(t, summary.Ports, 1)
	})

	t.Run("port diagnostics severity filter", func(t *testing.T) {
		w := do(t, "/api/v1/debug/port-diagnostics?severity=warning")
		require.Equal(t, http.StatusOK, w.Code)
		var summary debug.PortDiagnosticsSummary
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
		for _, p := range summary.Ports {
			assert.Equal(t, "warning", string(p.Overall))
		}
	})

	t.Run("single port diagnostic", func(t *testing.T) {
		w := do(t, "/api/v1/debug/port-diagnostics/"+lspUUID)
		require.Equal(t, http.StatusOK, w.Code)
		var diag debug.PortDiagnostic
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &diag))
		assert.Equal(t, "port-dbg", diag.PortName)
	})

	t.Run("single port diagnostic not found", func(t *testing.T) {
		w := do(t, "/api/v1/debug/port-diagnostics/does-not-exist")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("connectivity", func(t *testing.T) {
		w := do(t, "/api/v1/debug/connectivity?src="+lspUUID+"&dst="+lspUUID2)
		require.Equal(t, http.StatusOK, w.Code)
		var result debug.ConnectivityResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		assert.NotEmpty(t, result.Checks)
	})

	t.Run("acl audit", func(t *testing.T) {
		w := do(t, "/api/v1/debug/acl-audit")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("stale entries", func(t *testing.T) {
		w := do(t, "/api/v1/debug/stale-entries")
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandlePortDiagnostics_InvalidParams(t *testing.T) {
	// Both params are validated before the diagnoser is touched, so a nil
	// diagnoser is sufficient to exercise the 400 paths.
	tests := []struct {
		name string
		url  string
	}{
		{"invalid severity", "/api/v1/debug/port-diagnostics?severity=critical"},
		{"non-numeric limit", "/api/v1/debug/port-diagnostics?limit=abc"},
		{"negative limit", "/api/v1/debug/port-diagnostics?limit=-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handlePortDiagnostics(nil)
			req, err := http.NewRequestWithContext(context.Background(), "GET", tt.url, nil)
			require.NoError(t, err)
			w := httptest.NewRecorder()

			handler(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleConnectivity_MissingParams(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/api/v1/debug/connectivity"},
		{"missing dst", "/api/v1/debug/connectivity?src=abc"},
		{"missing src", "/api/v1/debug/connectivity?dst=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleConnectivity(nil) // checker not needed for param validation
			req, err := http.NewRequestWithContext(context.Background(), "GET", tt.url, nil)
			require.NoError(t, err)
			w := httptest.NewRecorder()

			handler(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}
