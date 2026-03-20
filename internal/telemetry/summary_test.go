package telemetry

import (
	"context"
	"testing"

	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuerier_Summary(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 10, 9, 8)
	testutil.InsertLogicalSwitch(t, nbClient, "test-sw")
	testutil.InsertSBGlobal(t, sbClient, 9)
	testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")

	querier := NewQuerier(nbClient, sbClient)
	result, err := querier.Summary(context.Background())
	require.NoError(t, err)

	assert.True(t, result.Connected["nb"])
	assert.True(t, result.Connected["sb"])
	assert.Equal(t, 1, result.Counts["logical_switches"])
	assert.Equal(t, 1, result.Counts["chassis"])
	require.NotNil(t, result.Propagation)
	assert.Equal(t, 10, result.Propagation.NbCfg)
	assert.Equal(t, 9, result.Propagation.SbNbCfg)
}

func TestQuerier_FlowMetrics(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	// No flows inserted — should return zero counts
	querier := NewQuerier(nbClient, sbClient)
	result, err := querier.FlowMetrics(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
}

func TestQuerier_Propagation(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	testutil.InsertNBGlobal(t, nbClient, 5, 4, 3)
	testutil.InsertSBGlobal(t, sbClient, 4)
	testutil.InsertChassis(t, sbClient, "ch-1", "host-1", "10.0.0.1")

	querier := NewQuerier(nbClient, sbClient)
	result, err := querier.Propagation(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 5, result.NbCfg)
	assert.Equal(t, 4, result.SbNbCfg)
	assert.Equal(t, 3, result.HvCfg)
	require.Len(t, result.Chassis, 1)
	assert.Equal(t, "ch-1", result.Chassis[0].Name)
	assert.Equal(t, 5, result.Chassis[0].Lag) // NbCfg(5) - Chassis.NbCfg(0) = 5
}

func TestQuerier_Cluster(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	querier := NewQuerier(nbClient, sbClient)
	result, err := querier.Cluster(context.Background())
	require.NoError(t, err)

	assert.True(t, result.Connected["nb"])
	assert.True(t, result.Connected["sb"])
	assert.NotNil(t, result.Connections)
}
