package enrich

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestCACertHTTPClient(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := caCertHTTPClient(filepath.Join(t.TempDir(), "nope.pem"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading OpenStack CA cert")
	})

	t.Run("non-PEM content", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "garbage.pem")
		require.NoError(t, os.WriteFile(path, []byte("not a certificate"), 0o600))

		_, err := caCertHTTPClient(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no PEM certificates")
	})

	t.Run("valid CA cert", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "ca.pem")
		require.NoError(t, os.WriteFile(path, generateTestCAPEM(t), 0o600))

		client, err := caCertHTTPClient(path)
		require.NoError(t, err)
		require.NotNil(t, client)

		tr, ok := client.Transport.(*http.Transport)
		require.True(t, ok, "transport should be *http.Transport")
		require.NotNil(t, tr.TLSClientConfig)
		require.NotNil(t, tr.TLSClientConfig.RootCAs, "custom CA pool should be set")
		assert.Equal(t, uint16(tls.VersionTLS12), tr.TLSClientConfig.MinVersion)
	})
}

// generateTestCAPEM returns a self-signed CA certificate in PEM form. Times are
// fixed (no time.Now) so the test is deterministic.
func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "northwatch-test-ca"},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Unix(1<<31-1, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
