package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Defaults(t *testing.T) {
	cfg, err := Parse([]string{"--ovn-nb-addr", "tcp:127.0.0.1:6641", "--ovn-sb-addr", "tcp:127.0.0.1:6642"})
	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.Listen)
	assert.Equal(t, "tcp:127.0.0.1:6641", cfg.OVNNBAddr)
	assert.Equal(t, "tcp:127.0.0.1:6642", cfg.OVNSBAddr)
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
