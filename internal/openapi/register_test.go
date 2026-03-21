package openapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSpec_HasExpectedPaths(t *testing.T) {
	doc := BuildSpec()

	assert.Equal(t, "3.1.0", doc.OpenAPI)
	assert.Equal(t, "Northwatch API", doc.Info.Title)

	// Verify key endpoints exist
	expectedPaths := []string{
		"/healthz",
		"/readyz",
		"/api/v1/capabilities",
		"/api/v1/nb/logical-switches",
		"/api/v1/nb/logical-switches/{uuid}",
		"/api/v1/nb/logical-routers",
		"/api/v1/nb/acls",
		"/api/v1/sb/chassis",
		"/api/v1/sb/port-bindings",
		"/api/v1/sb/logical-flows",
		"/api/v1/correlated/logical-switches",
		"/api/v1/correlated/chassis",
		"/api/v1/search",
		"/api/v1/topology",
		"/api/v1/flows",
		"/api/v1/debug/port-diagnostics",
		"/api/v1/debug/connectivity",
		"/api/v1/debug/trace",
		"/api/v1/debug/flow-diff",
		"/api/v1/snapshots",
		"/api/v1/snapshots/{id}",
		"/api/v1/snapshots/{id}/rows",
		"/api/v1/snapshots/{id}/export",
		"/api/v1/snapshots/diff",
		"/api/v1/snapshots/import",
		"/api/v1/events",
		"/api/v1/alerts",
		"/api/v1/alerts/rules",
		"/api/v1/alerts/silences",
		"/api/v1/telemetry/summary",
		"/api/v1/telemetry/flows",
		"/api/v1/telemetry/propagation",
		"/api/v1/telemetry/cluster",
		"/metrics",
		"/api/v1/ws",
	}

	for _, path := range expectedPaths {
		assert.Contains(t, doc.Paths, path, "missing path: %s", path)
	}
}

func TestBuildSpec_HasComponentSchemas(t *testing.T) {
	doc := BuildSpec()

	require.NotNil(t, doc.Components)
	require.NotEmpty(t, doc.Components.Schemas)

	// Check key schemas exist
	expectedSchemas := []string{
		"LogicalSwitch",
		"LogicalSwitchPort",
		"LogicalRouter",
		"ACL",
		"NAT",
		"Chassis",
		"PortBinding",
		"DatapathBinding",
	}

	for _, name := range expectedSchemas {
		assert.Contains(t, doc.Components.Schemas, name, "missing schema: %s", name)
	}
}

func TestBuildSpec_ValidJSON(t *testing.T) {
	doc := BuildSpec()
	data, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	assert.True(t, len(data) > 1000, "spec should be substantial, got %d bytes", len(data))
}

func TestBuildSpec_UniqueOperationIDs(t *testing.T) {
	doc := BuildSpec()
	seen := make(map[string]string) // operationID → path

	for path, pi := range doc.Paths {
		for _, op := range []*Operation{pi.Get, pi.Post, pi.Put, pi.Delete} {
			if op == nil || op.OperationID == "" {
				continue
			}
			if prev, exists := seen[op.OperationID]; exists {
				t.Errorf("duplicate operationId %q: %s and %s", op.OperationID, prev, path)
			}
			seen[op.OperationID] = path
		}
	}
}
