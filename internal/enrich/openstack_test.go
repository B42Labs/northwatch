package enrich

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider() *OpenStackProvider {
	return &OpenStackProvider{
		LookupServer: func(_ context.Context, serverID string) (string, error) {
			if serverID == "server-123" {
				return "my-vm", nil
			}
			return "", fmt.Errorf("not found")
		},
		LookupProject: func(_ context.Context, projectID string) (string, error) {
			if projectID == "project-456" {
				return "my-project", nil
			}
			return "", fmt.Errorf("not found")
		},
	}
}

func TestOpenStackProvider_EnrichPort(t *testing.T) {
	p := newTestProvider()

	t.Run("full enrichment", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"neutron:port_name":    "my-port",
			"neutron:device_owner": "compute:nova",
			"neutron:device_id":    "server-123",
			"neutron:project_id":   "project-456",
		})

		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-port", info.DisplayName)
		assert.Equal(t, "compute:nova", info.DeviceOwner)
		assert.Equal(t, "server-123", info.DeviceID)
		assert.Equal(t, "my-vm", info.DeviceName)
		assert.Equal(t, "project-456", info.ProjectID)
		assert.Equal(t, "my-project", info.ProjectName)
	})

	t.Run("no external_ids", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("partial data", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"neutron:port_name": "simple-port",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "simple-port", info.DisplayName)
		assert.Empty(t, info.DeviceName)
	})

	t.Run("non-compute device", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"neutron:port_name":    "dhcp-port",
			"neutron:device_owner": "network:dhcp",
			"neutron:device_id":    "dhcp-agent-id",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Empty(t, info.DeviceName, "should not look up non-compute devices via Nova")
	})

	t.Run("server lookup failure", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"neutron:port_name":    "my-port",
			"neutron:device_owner": "compute:nova",
			"neutron:device_id":    "unknown-server",
			"neutron:project_id":   "project-456",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Empty(t, info.DeviceName, "should gracefully handle lookup failure")
		assert.Equal(t, "my-project", info.ProjectName)
	})
}

func TestOpenStackProvider_EnrichNetwork(t *testing.T) {
	p := newTestProvider()

	t.Run("with data", func(t *testing.T) {
		info, err := p.EnrichNetwork(context.Background(), map[string]string{
			"neutron:network_name": "my-network",
			"neutron:project_id":   "project-456",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-network", info.DisplayName)
		assert.Equal(t, "my-project", info.ProjectName)
	})

	t.Run("empty", func(t *testing.T) {
		info, err := p.EnrichNetwork(context.Background(), map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info)
	})
}

func TestOpenStackProvider_EnrichRouter(t *testing.T) {
	p := newTestProvider()

	info, err := p.EnrichRouter(context.Background(), map[string]string{
		"neutron:router_name": "my-router",
		"neutron:project_id":  "project-456",
	})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "my-router", info.DisplayName)
	assert.Equal(t, "my-project", info.ProjectName)
}

func TestOpenStackProvider_EnrichNAT(t *testing.T) {
	p := newTestProvider()

	t.Run("with fip data", func(t *testing.T) {
		info, err := p.EnrichNAT(context.Background(), map[string]string{
			"neutron:fip_id":           "fip-789",
			"neutron:fip_external_mac": "fa:16:3e:aa:bb:cc",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "fip-789", info.Extra["fip_id"])
		assert.Equal(t, "fa:16:3e:aa:bb:cc", info.Extra["fip_external_mac"])
	})

	t.Run("no nat data", func(t *testing.T) {
		info, err := p.EnrichNAT(context.Background(), map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info)
	})
}

func TestOpenStackProvider_Name(t *testing.T) {
	p := &OpenStackProvider{}
	assert.Equal(t, "openstack", p.Name())
}
