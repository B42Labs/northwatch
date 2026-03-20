package config

import (
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
