package telemetry

import (
	"testing"

	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)

	// Insert test data into NB
	testutil.InsertNBGlobal(t, nbClient, 5, 4, 3)
	testutil.InsertLogicalSwitch(t, nbClient, "sw1")
	testutil.InsertLogicalSwitch(t, nbClient, "sw2")

	// Insert test data into SB
	testutil.InsertSBGlobal(t, sbClient, 4)
	chassisUUID := testutil.InsertChassis(t, sbClient, "chassis-1", "host-1", "10.0.0.1")
	testutil.InsertPortBinding(t, sbClient, "port-1", "", &chassisUUID)

	collector := NewCollector(nbClient, sbClient)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	families, err := registry.Gather()
	require.NoError(t, err)

	metrics := map[string]*dto.MetricFamily{}
	for _, f := range families {
		metrics[f.GetName()] = f
	}

	// Verify connection status
	connFam := metrics["northwatch_ovsdb_connected"]
	require.NotNil(t, connFam)
	assert.Len(t, connFam.GetMetric(), 2)

	// Verify NB_Global metrics
	nbCfgFam := metrics["northwatch_nb_cfg"]
	require.NotNil(t, nbCfgFam)
	assert.Equal(t, float64(5), nbCfgFam.GetMetric()[0].GetGauge().GetValue())

	sbCfgFam := metrics["northwatch_sb_cfg"]
	require.NotNil(t, sbCfgFam)
	assert.Equal(t, float64(4), sbCfgFam.GetMetric()[0].GetGauge().GetValue())

	hvCfgFam := metrics["northwatch_hv_cfg"]
	require.NotNil(t, hvCfgFam)
	assert.Equal(t, float64(3), hvCfgFam.GetMetric()[0].GetGauge().GetValue())

	// Verify SB_Global NbCfg
	sbNbCfgFam := metrics["northwatch_sb_nb_cfg"]
	require.NotNil(t, sbNbCfgFam)
	assert.Equal(t, float64(4), sbNbCfgFam.GetMetric()[0].GetGauge().GetValue())

	// Verify table row counts exist
	tableRowsFam := metrics["northwatch_ovsdb_table_rows"]
	require.NotNil(t, tableRowsFam)
	assert.Greater(t, len(tableRowsFam.GetMetric()), 0)

	// Verify chassis NbCfg lag
	lagFam := metrics["northwatch_chassis_nb_cfg_lag"]
	require.NotNil(t, lagFam)
	// NB_Global.NbCfg (5) - Chassis.NbCfg (0) = 5
	assert.Equal(t, float64(5), lagFam.GetMetric()[0].GetGauge().GetValue())
}
