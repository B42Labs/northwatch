package ovscorrelate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
)

func boolPtr(b bool) *bool { return &b }

func TestForInterface(t *testing.T) {
	sbClient := testutil.SetupSBTestClient(t)
	chassisUUID := testutil.InsertChassis(t, sbClient, "chassis-1", "host-1", "10.0.0.1")
	testutil.InsertPortBindingWithUp(t, sbClient, "lsp-a", "", &chassisUUID, boolPtr(true))

	cor := &Correlator{SB: sbClient}

	tests := []struct {
		name  string
		live  LiveInterface
		check func(t *testing.T, c Correlation)
	}{
		{
			name: "bound and up with a healthy interface has no drift",
			live: LiveInterface{SystemID: "chassis-1", IfaceID: "lsp-a", LinkState: "up"},
			check: func(t *testing.T, c Correlation) {
				assert.Equal(t, "lsp-a", c.IfaceID)
				assert.True(t, c.Bound)
				require.NotNil(t, c.Binding)
				assert.Equal(t, "lsp-a", c.Binding.LogicalPort)
				require.NotNil(t, c.Binding.Up)
				assert.True(t, *c.Binding.Up)
				assert.Equal(t, "chassis-1", c.Binding.Chassis)
				assert.True(t, c.Binding.BoundHere)
				// The testutil datapath carries no external_ids name, so the
				// label falls back to tunnel_key; the UUID reference is present.
				assert.NotEmpty(t, c.Binding.Datapath)
				assert.NotEmpty(t, c.Binding.DatapathUUID)
				assert.Empty(t, c.Drift)
			},
		},
		{
			name: "SB up but interface link down is flagged as drift",
			live: LiveInterface{SystemID: "chassis-1", IfaceID: "lsp-a", LinkState: "down"},
			check: func(t *testing.T, c Correlation) {
				require.True(t, c.Bound)
				require.Len(t, c.Drift, 1)
				assert.Contains(t, c.Drift[0], "link_state")
			},
		},
		{
			name: "SB up but interface erroring is flagged as drift",
			live: LiveInterface{SystemID: "chassis-1", IfaceID: "lsp-a", LinkState: "up", Error: "no carrier"},
			check: func(t *testing.T, c Correlation) {
				require.Len(t, c.Drift, 1)
				assert.Contains(t, c.Drift[0], "no carrier")
			},
		},
		{
			name: "port bound on a different chassis is not bound here",
			live: LiveInterface{SystemID: "other", IfaceID: "lsp-a", LinkState: "up"},
			check: func(t *testing.T, c Correlation) {
				require.NotNil(t, c.Binding)
				assert.Equal(t, "chassis-1", c.Binding.Chassis)
				assert.False(t, c.Binding.BoundHere)
			},
		},
		{
			name: "no port binding for the iface-id degrades to unbound",
			live: LiveInterface{SystemID: "chassis-1", IfaceID: "absent", LinkState: "up"},
			check: func(t *testing.T, c Correlation) {
				assert.Equal(t, "absent", c.IfaceID)
				assert.False(t, c.Bound)
				assert.Nil(t, c.Binding)
				assert.Empty(t, c.Drift)
			},
		},
		{
			name: "empty iface-id means the interface is not OVN-managed",
			live: LiveInterface{SystemID: "chassis-1", IfaceID: "", LinkState: "up"},
			check: func(t *testing.T, c Correlation) {
				assert.Empty(t, c.IfaceID)
				assert.False(t, c.Bound)
				assert.Nil(t, c.Binding)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cor.ForInterface(context.Background(), tt.live)
			require.NoError(t, err)
			tt.check(t, got)
		})
	}
}

func TestDatapathLabel(t *testing.T) {
	tests := []struct {
		name string
		dp   sb.DatapathBinding
		want string
	}{
		{
			name: "prefers the external_ids name",
			dp:   sb.DatapathBinding{ExternalIDs: map[string]string{"name": "sw0", "name2": "alt"}, TunnelKey: 7},
			want: "sw0",
		},
		{
			name: "falls back to name2 when name is absent",
			dp:   sb.DatapathBinding{ExternalIDs: map[string]string{"name2": "sw0-alt"}, TunnelKey: 7},
			want: "sw0-alt",
		},
		{
			name: "falls back to the tunnel key when unnamed",
			dp:   sb.DatapathBinding{ExternalIDs: map[string]string{}, TunnelKey: 42},
			want: "tunnel_key 42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, datapathLabel(&tt.dp))
		})
	}
}
