package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Defaults(t *testing.T) {
	cfg, err := Parse([]string{"--ovn-nb-addr", "tcp:127.0.0.1:6641", "--ovn-sb-addr", "tcp:127.0.0.1:6642"})
	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.Listen)
	assert.Equal(t, "tcp:127.0.0.1:6641", cfg.OVNNBAddr)
	assert.Equal(t, "tcp:127.0.0.1:6642", cfg.OVNSBAddr)
	assert.Equal(t, 5*time.Minute, cfg.EnrichmentCacheTTL)
}

func TestParse_CustomListen(t *testing.T) {
	cfg, err := Parse([]string{"--listen", ":9090", "--ovn-nb-addr", "tcp:10.0.0.1:6641", "--ovn-sb-addr", "tcp:10.0.0.1:6642"})
	require.NoError(t, err)
	assert.Equal(t, ":9090", cfg.Listen)
}

func TestParse_MissingNBAddr(t *testing.T) {
	_, err := Parse([]string{"--ovn-sb-addr", "tcp:127.0.0.1:6642"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ovn-nb-addr")
}

func TestParse_MissingSBAddr(t *testing.T) {
	_, err := Parse([]string{"--ovn-nb-addr", "tcp:127.0.0.1:6641"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ovn-sb-addr")
}

func TestParse_EnvOverride(t *testing.T) {
	t.Setenv("NORTHWATCH_LISTEN", ":3000")
	t.Setenv("NORTHWATCH_OVN_NB_ADDR", "tcp:env-nb:6641")
	t.Setenv("NORTHWATCH_OVN_SB_ADDR", "tcp:env-sb:6642")

	cfg, err := Parse([]string{})
	require.NoError(t, err)
	assert.Equal(t, ":3000", cfg.Listen)
	assert.Equal(t, "tcp:env-nb:6641", cfg.OVNNBAddr)
	assert.Equal(t, "tcp:env-sb:6642", cfg.OVNSBAddr)
}

func TestParse_FlagOverridesEnv(t *testing.T) {
	t.Setenv("NORTHWATCH_OVN_NB_ADDR", "tcp:env:6641")
	t.Setenv("NORTHWATCH_OVN_SB_ADDR", "tcp:env:6642")

	cfg, err := Parse([]string{"--ovn-nb-addr", "tcp:flag:6641", "--ovn-sb-addr", "tcp:flag:6642"})
	require.NoError(t, err)
	assert.Equal(t, "tcp:flag:6641", cfg.OVNNBAddr)
	assert.Equal(t, "tcp:flag:6642", cfg.OVNSBAddr)
}

func TestParse_OpenStackFlags(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--os-auth-url", "https://keystone:5000/v3",
		"--os-username", "admin",
		"--os-password", "secret",
		"--os-project-name", "admin",
		"--os-domain-name", "Default",
		"--os-region-name", "RegionOne",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://keystone:5000/v3", cfg.OpenStackAuthURL)
	assert.Equal(t, "admin", cfg.OpenStackUsername)
	assert.Equal(t, "secret", cfg.OpenStackPassword)
	assert.Equal(t, "admin", cfg.OpenStackProjectName)
	assert.Equal(t, "Default", cfg.OpenStackDomainName)
	assert.Equal(t, "RegionOne", cfg.OpenStackRegionName)
}

func TestParse_OpenStackEnvVars(t *testing.T) {
	t.Setenv("NORTHWATCH_OVN_NB_ADDR", "tcp:127.0.0.1:6641")
	t.Setenv("NORTHWATCH_OVN_SB_ADDR", "tcp:127.0.0.1:6642")
	t.Setenv("OS_AUTH_URL", "https://keystone:5000/v3")
	t.Setenv("OS_USERNAME", "admin")

	cfg, err := Parse([]string{})
	require.NoError(t, err)
	assert.Equal(t, "https://keystone:5000/v3", cfg.OpenStackAuthURL)
	assert.Equal(t, "admin", cfg.OpenStackUsername)
}

func TestParse_CustomCacheTTL(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--enrichment-cache-ttl", "10m",
	})
	require.NoError(t, err)
	assert.Equal(t, 10*time.Minute, cfg.EnrichmentCacheTTL)
}

func TestParse_InvalidCacheTTL(t *testing.T) {
	_, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--enrichment-cache-ttl", "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrichment-cache-ttl")
}

func TestParse_NoOpenStack(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.OpenStackAuthURL)
	assert.Empty(t, cfg.OpenStackUsername)
}

func TestParse_HistoryDefaults(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
	})
	require.NoError(t, err)
	assert.Equal(t, "northwatch-history.db", cfg.HistoryDBPath)
	assert.Equal(t, 5*time.Minute, cfg.SnapshotInterval)
	assert.Equal(t, 24*time.Hour, cfg.EventRetention)
}

func TestParse_HistoryFlags(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--history-db-path", "/tmp/test.db",
		"--snapshot-interval", "10m",
		"--event-retention", "48h",
	})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test.db", cfg.HistoryDBPath)
	assert.Equal(t, 10*time.Minute, cfg.SnapshotInterval)
	assert.Equal(t, 48*time.Hour, cfg.EventRetention)
}

func TestParse_HistoryEnvVars(t *testing.T) {
	t.Setenv("NORTHWATCH_OVN_NB_ADDR", "tcp:127.0.0.1:6641")
	t.Setenv("NORTHWATCH_OVN_SB_ADDR", "tcp:127.0.0.1:6642")
	t.Setenv("NORTHWATCH_HISTORY_DB_PATH", "/data/history.db")
	t.Setenv("NORTHWATCH_SNAPSHOT_INTERVAL", "15m")
	t.Setenv("NORTHWATCH_EVENT_RETENTION", "72h")

	cfg, err := Parse([]string{})
	require.NoError(t, err)
	assert.Equal(t, "/data/history.db", cfg.HistoryDBPath)
	assert.Equal(t, 15*time.Minute, cfg.SnapshotInterval)
	assert.Equal(t, 72*time.Hour, cfg.EventRetention)
}

func TestParse_InvalidSnapshotInterval(t *testing.T) {
	_, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--snapshot-interval", "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot-interval")
}

func TestParse_InvalidEventRetention(t *testing.T) {
	_, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--event-retention", "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event-retention")
}

func TestParse_SynthesizesSingleCluster(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
	})
	require.NoError(t, err)
	require.Len(t, cfg.Clusters, 1)
	assert.Equal(t, "default", cfg.Clusters[0].Name)
	assert.Equal(t, "Default", cfg.Clusters[0].Label)
	assert.Equal(t, "tcp:127.0.0.1:6641", cfg.Clusters[0].OVNNBAddr)
	assert.Equal(t, "tcp:127.0.0.1:6642", cfg.Clusters[0].OVNSBAddr)
	assert.Nil(t, cfg.Clusters[0].Enrichment)
}

func TestParse_SynthesizesClusterWithOpenStackEnrichment(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--os-auth-url", "https://keystone:5000/v3",
		"--os-username", "admin",
	})
	require.NoError(t, err)
	require.Len(t, cfg.Clusters, 1)
	require.NotNil(t, cfg.Clusters[0].Enrichment)
	assert.Equal(t, "openstack", cfg.Clusters[0].Enrichment.Type)
	assert.Equal(t, "https://keystone:5000/v3", cfg.Clusters[0].Enrichment.OpenStackAuthURL)
}

func TestParse_SynthesizesClusterWithKubeEnrichment(t *testing.T) {
	cfg, err := Parse([]string{
		"--ovn-nb-addr", "tcp:127.0.0.1:6641",
		"--ovn-sb-addr", "tcp:127.0.0.1:6642",
		"--kube-enrichment",
		"--kubeconfig", "/home/user/.kube/config",
	})
	require.NoError(t, err)
	require.Len(t, cfg.Clusters, 1)
	require.NotNil(t, cfg.Clusters[0].Enrichment)
	assert.Equal(t, "kubernetes", cfg.Clusters[0].Enrichment.Type)
	assert.Equal(t, "/home/user/.kube/config", cfg.Clusters[0].Enrichment.Kubeconfig)
}

func TestParse_ConfigFile(t *testing.T) {
	content := `{
		"clusters": [
			{
				"name": "prod",
				"label": "Production",
				"ovn_nb_addr": "tcp:10.0.0.1:6641",
				"ovn_sb_addr": "tcp:10.0.0.1:6642"
			},
			{
				"name": "staging",
				"ovn_nb_addr": "tcp:10.0.0.2:6641",
				"ovn_sb_addr": "tcp:10.0.0.2:6642"
			}
		]
	}`
	path := writeTestFile(t, content)

	cfg, err := Parse([]string{"--config-file", path})
	require.NoError(t, err)
	require.Len(t, cfg.Clusters, 2)

	assert.Equal(t, "prod", cfg.Clusters[0].Name)
	assert.Equal(t, "Production", cfg.Clusters[0].Label)
	assert.Equal(t, "tcp:10.0.0.1:6641", cfg.Clusters[0].OVNNBAddr)
	assert.Equal(t, "tcp:10.0.0.1:6642", cfg.Clusters[0].OVNSBAddr)

	assert.Equal(t, "staging", cfg.Clusters[1].Name)
	assert.Equal(t, "staging", cfg.Clusters[1].Label) // label defaults to name
	assert.Equal(t, "tcp:10.0.0.2:6641", cfg.Clusters[1].OVNNBAddr)
}

func TestParse_ConfigFile_WithEnrichment(t *testing.T) {
	content := `{
		"clusters": [
			{
				"name": "prod",
				"ovn_nb_addr": "tcp:10.0.0.1:6641",
				"ovn_sb_addr": "tcp:10.0.0.1:6642",
				"enrichment": {
					"type": "openstack",
					"os_auth_url": "https://keystone:5000/v3",
					"os_username": "admin"
				}
			}
		]
	}`
	path := writeTestFile(t, content)

	cfg, err := Parse([]string{"--config-file", path})
	require.NoError(t, err)
	require.Len(t, cfg.Clusters, 1)
	require.NotNil(t, cfg.Clusters[0].Enrichment)
	assert.Equal(t, "openstack", cfg.Clusters[0].Enrichment.Type)
	assert.Equal(t, "https://keystone:5000/v3", cfg.Clusters[0].Enrichment.OpenStackAuthURL)
}

func TestParse_ConfigFile_Empty(t *testing.T) {
	content := `{"clusters": []}`
	path := writeTestFile(t, content)

	_, err := Parse([]string{"--config-file", path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one cluster")
}

func TestParse_ConfigFile_MissingName(t *testing.T) {
	content := `{"clusters": [{"ovn_nb_addr": "tcp:10.0.0.1:6641", "ovn_sb_addr": "tcp:10.0.0.1:6642"}]}`
	path := writeTestFile(t, content)

	_, err := Parse([]string{"--config-file", path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestParse_ConfigFile_MissingNBAddr(t *testing.T) {
	content := `{"clusters": [{"name": "prod", "ovn_sb_addr": "tcp:10.0.0.1:6642"}]}`
	path := writeTestFile(t, content)

	_, err := Parse([]string{"--config-file", path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ovn_nb_addr is required")
}

func TestParse_ConfigFile_MissingSBAddr(t *testing.T) {
	content := `{"clusters": [{"name": "prod", "ovn_nb_addr": "tcp:10.0.0.1:6641"}]}`
	path := writeTestFile(t, content)

	_, err := Parse([]string{"--config-file", path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ovn_sb_addr is required")
}

func TestParse_ConfigFile_InvalidJSON(t *testing.T) {
	content := `{not valid json}`
	path := writeTestFile(t, content)

	_, err := Parse([]string{"--config-file", path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing")
}

func TestParse_ConfigFile_NotFound(t *testing.T) {
	_, err := Parse([]string{"--config-file", "/nonexistent/config.json"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
}

func TestParse_ConfigFile_NoBAddrValidation(t *testing.T) {
	// When using config file, NB/SB addresses are not required via flags
	content := `{
		"clusters": [
			{
				"name": "prod",
				"ovn_nb_addr": "tcp:10.0.0.1:6641",
				"ovn_sb_addr": "tcp:10.0.0.1:6642"
			}
		]
	}`
	path := writeTestFile(t, content)

	cfg, err := Parse([]string{"--config-file", path})
	require.NoError(t, err)
	assert.Empty(t, cfg.OVNNBAddr) // flat flag is empty
	assert.Len(t, cfg.Clusters, 1)  // but clusters are populated
}

func TestParse_ConfigFileEnv(t *testing.T) {
	content := `{
		"clusters": [
			{
				"name": "env-cluster",
				"ovn_nb_addr": "tcp:10.0.0.1:6641",
				"ovn_sb_addr": "tcp:10.0.0.1:6642"
			}
		]
	}`
	path := writeTestFile(t, content)
	t.Setenv("NORTHWATCH_CONFIG_FILE", path)

	cfg, err := Parse([]string{})
	require.NoError(t, err)
	require.Len(t, cfg.Clusters, 1)
	assert.Equal(t, "env-cluster", cfg.Clusters[0].Name)
}

func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}
