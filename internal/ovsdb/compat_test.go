package ovsdb

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
	libovsdb "github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaFloor pins the schemas the models are generated from to the
// documented minimum releases: OVN 24.03 ships NB 7.3.0 / SB 20.33.0, Open
// vSwitch 3.3 ships Open_vSwitch 8.5.0. Raising OVN_VERSION or OVS_VERSION in
// the Makefile drops support for every older release, so it fails here until
// MinOVNVersion / MinOVSVersion and docs/reference/compatibility.md are updated
// with it.
func TestSchemaFloor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		schema  libovsdb.DatabaseSchema
		release string
		want    string
	}{
		{"northbound", nb.Schema(), "OVN " + MinOVNVersion, "7.3.0"},
		{"southbound", sb.Schema(), "OVN " + MinOVNVersion, "20.33.0"},
		{"open_vswitch", vs.Schema(), "Open vSwitch " + MinOVSVersion, "8.5.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tt.want, tt.schema.Version,
				"%s models are no longer generated from the %s schema", tt.schema.Name, tt.release)
		})
	}
}

// TestModelsValidateAgainstSupportedSchemas runs libovsdb's connect-time
// validation of the generated models against the floor schemas and against
// every newer release kept in testdata/schemas. Drop a newly released schema
// into that directory to extend the tested range.
func TestModelsValidateAgainstSupportedSchemas(t *testing.T) {
	t.Parallel()

	models := map[string]func() (model.ClientDBModel, error){
		nb.Schema().Name: nb.FullDatabaseModel,
		sb.Schema().Name: sb.FullDatabaseModel,
		vs.Schema().Name: vs.FullDatabaseModel,
	}

	type schemaCase struct {
		name   string
		schema libovsdb.DatabaseSchema
	}
	cases := []schemaCase{
		{"floor/" + nb.Schema().Name, nb.Schema()},
		{"floor/" + sb.Schema().Name, sb.Schema()},
		{"floor/" + vs.Schema().Name, vs.Schema()},
	}

	files, err := filepath.Glob(filepath.Join("testdata", "schemas", "*.ovsschema"))
	require.NoError(t, err)
	newer := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f) // #nosec G304 -- path from a glob over testdata/, not input
		require.NoError(t, err)
		var schema libovsdb.DatabaseSchema
		require.NoErrorf(t, json.Unmarshal(raw, &schema), "parsing %s", f)
		newer[schema.Name] = true
		cases = append(cases, schemaCase{filepath.Base(f), schema})
	}
	for name := range models {
		require.Truef(t, newer[name], "testdata/schemas holds no newer %s schema", name)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			newModel, ok := models[tc.schema.Name]
			require.Truef(t, ok, "no generated model for database %q", tc.schema.Name)
			clientModel, err := newModel()
			require.NoError(t, err)

			_, errs := model.NewDatabaseModel(tc.schema, clientModel)
			assert.Emptyf(t, errs, "models do not validate against %s %s", tc.schema.Name, tc.schema.Version)
		})
	}
}

func TestWithSchemaHint(t *testing.T) {
	t.Parallel()

	validation := errors.New("failed to connect to tcp:127.0.0.1:6641: database OVN_Northbound validation error (1): " +
		"database model contains a model for table Sample that does not exist in schema")
	refused := errors.New("failed to connect to tcp:127.0.0.1:6641: connection refused")

	tests := []struct {
		name     string
		err      error
		product  string
		version  string
		wantHint string // empty: the error must come back unchanged
	}{
		{"nil", nil, "OVN", MinOVNVersion, ""},
		{"other error", refused, "OVN", MinOVNVersion, ""},
		{"ovn validation error", validation, "OVN", MinOVNVersion, "incompatible OVN schema (Northwatch supports OVN 24.03 and newer"},
		{"ovs validation error", validation, "Open vSwitch", MinOVSVersion, "incompatible Open vSwitch schema (Northwatch supports Open vSwitch 3.3 and newer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := withSchemaHint(tt.err, tt.product, tt.version)
			if tt.wantHint == "" {
				assert.Equal(t, tt.err, got)
				return
			}
			require.ErrorIs(t, got, tt.err)
			assert.Contains(t, got.Error(), tt.wantHint)
			assert.Contains(t, got.Error(), compatibilityDocsURL)
		})
	}
}

// nbGlobalFromTheFuture models NB_Global with a column the server's schema
// lacks, the way models generated from a newer OVN release would.
type nbGlobalFromTheFuture struct {
	UUID   string  `ovsdb:"_uuid"`
	Future *string `ovsdb:"future_column"`
}

// TestConnect_IncompatibleSchemaHint pins the libovsdb error text withSchemaHint
// matches: a model the server's schema cannot satisfy must fail Connect with
// the supported-version hint, not only libovsdb's list of missing columns.
func TestConnect_IncompatibleSchemaHint(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	nbModel, err := model.NewClientDBModel(nb.Schema().Name, map[string]model.Model{
		nb.NBGlobalTable: &nbGlobalFromTheFuture{},
	})
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = Connect(ctx, nbAddr, sbAddr, nbModel, sbModel, MonitorOptions{}, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connecting to NB")
	assert.Contains(t, err.Error(), "incompatible OVN schema (Northwatch supports OVN "+MinOVNVersion+" and newer")
	assert.Contains(t, err.Error(), "future_column")
}

// ovsFromTheFuture models the Open_vSwitch root table with a column the
// chassis' schema lacks.
type ovsFromTheFuture struct {
	UUID   string  `ovsdb:"_uuid"`
	Future *string `ovsdb:"future_column"`
}

// TestOVSPool_IncompatibleSchemaHint covers the per-chassis path: a chassis
// whose Open_vSwitch schema is older than the models fails its connection
// attempt with the Open vSwitch version hint.
func TestOVSPool_IncompatibleSchemaHint(t *testing.T) {
	sock, _ := testutil.SetupOVSTestServer(t)

	ovsModel, err := model.NewClientDBModel(vs.Schema().Name, map[string]model.Model{
		vs.OpenvSwitchTable: &ovsFromTheFuture{},
	})
	require.NoError(t, err)
	pool := NewOVSPool(ovsModel, nil)
	defer pool.Close()

	c, err := client.NewOVSDBClient(ovsModel, client.WithEndpoint("unix:"+sock))
	require.NoError(t, err)
	defer c.Close()

	err = pool.connectAndMonitor(&ovsMember{systemID: "chassis-1", addr: "unix:" + sock, client: c})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incompatible Open vSwitch schema (Northwatch supports Open vSwitch "+MinOVSVersion+" and newer")
	assert.Contains(t, err.Error(), "future_column")
}
