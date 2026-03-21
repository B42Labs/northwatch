package enrich

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestKubernetesProvider() *KubernetesProvider {
	return &KubernetesProvider{
		LookupPod: func(_ context.Context, namespace, name string) (*PodInfo, error) {
			if namespace == "default" && name == "nginx-abc123" {
				return &PodInfo{
					Name:      "nginx-abc123",
					Namespace: "default",
					Labels:    map[string]string{"app": "nginx"},
					NodeName:  "node-1",
				}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		LookupService: func(_ context.Context, namespace, name string) (*ServiceInfo, error) {
			if namespace == "default" && name == "nginx-svc" {
				return &ServiceInfo{
					Name:      "nginx-svc",
					Namespace: "default",
					ClusterIP: "10.96.0.10",
				}, nil
			}
			return nil, fmt.Errorf("not found")
		},
	}
}

func TestKubernetesProvider_EnrichPort(t *testing.T) {
	p := newTestKubernetesProvider()

	t.Run("full enrichment", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"k8s.ovn.org/pod": "default/nginx-abc123",
			"k8s.ovn.org/nad": "default/my-network",
		})

		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "nginx-abc123", info.DisplayName)
		assert.Equal(t, "default", info.ProjectName)
		assert.Equal(t, "node-1", info.Extra["node"])
		assert.Equal(t, "nginx", info.Extra["label:app"])
		assert.Equal(t, "default/my-network", info.Extra["nad"])
	})

	t.Run("no external_ids", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("partial data without nad", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"k8s.ovn.org/pod": "default/nginx-abc123",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "nginx-abc123", info.DisplayName)
		assert.Equal(t, "default", info.ProjectName)
		assert.Equal(t, "node-1", info.Extra["node"])
		_, hasNAD := info.Extra["nad"]
		assert.False(t, hasNAD)
	})

	t.Run("pod lookup failure", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"k8s.ovn.org/pod": "kube-system/unknown-pod",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "unknown-pod", info.DisplayName)
		assert.Equal(t, "kube-system", info.ProjectName)
		assert.Empty(t, info.Extra["node"], "should gracefully handle lookup failure")
	})

	t.Run("nil LookupPod", func(t *testing.T) {
		p2 := &KubernetesProvider{}
		info, err := p2.EnrichPort(context.Background(), map[string]string{
			"k8s.ovn.org/pod": "default/nginx-abc123",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "nginx-abc123", info.DisplayName)
		assert.Equal(t, "default", info.ProjectName)
	})

	t.Run("invalid pod reference", func(t *testing.T) {
		info, err := p.EnrichPort(context.Background(), map[string]string{
			"k8s.ovn.org/pod": "invalid-no-slash",
		})
		require.NoError(t, err)
		assert.Nil(t, info)
	})
}

func TestKubernetesProvider_EnrichNetwork(t *testing.T) {
	p := newTestKubernetesProvider()

	t.Run("with data", func(t *testing.T) {
		info, err := p.EnrichNetwork(context.Background(), map[string]string{
			"k8s.ovn.org/network": "tenant-net",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "tenant-net", info.DisplayName)
	})

	t.Run("empty", func(t *testing.T) {
		info, err := p.EnrichNetwork(context.Background(), map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info)
	})
}

func TestKubernetesProvider_EnrichRouter(t *testing.T) {
	p := newTestKubernetesProvider()

	t.Run("with data", func(t *testing.T) {
		info, err := p.EnrichRouter(context.Background(), map[string]string{
			"k8s.ovn.org/network": "cluster-router",
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "cluster-router", info.DisplayName)
	})

	t.Run("empty", func(t *testing.T) {
		info, err := p.EnrichRouter(context.Background(), map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info)
	})
}

func TestKubernetesProvider_EnrichNAT(t *testing.T) {
	p := newTestKubernetesProvider()

	info, err := p.EnrichNAT(context.Background(), map[string]string{
		"some-key": "some-value",
	})
	require.NoError(t, err)
	assert.Nil(t, info, "NAT enrichment should always return nil for Kubernetes")
}

func TestKubernetesProvider_Name(t *testing.T) {
	p := &KubernetesProvider{}
	assert.Equal(t, "kubernetes", p.Name())
}
