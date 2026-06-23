package telemetry

import (
	"context"
	"testing"

	"github.com/ovn-kubernetes/libovsdb/ovsdb/serverdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/b42labs/northwatch/internal/testutil"
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

func TestQuerier_RaftHealth(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	sbClient := testutil.SetupSBTestClient(t)
	querier := NewQuerier(nbClient, sbClient)

	// Without any _Server monitor the cluster view degrades to "unavailable",
	// but the client-connection state is still reported and slices are non-nil.
	res, err := querier.RaftHealth(context.Background())
	require.NoError(t, err)
	assert.True(t, res.NB.ClientConnected)
	assert.True(t, res.SB.ClientConnected)
	assert.False(t, res.NB.Cluster.Available)
	assert.Equal(t, 0, res.NB.Cluster.Total)
	assert.NotNil(t, res.NB.Cluster.Members)
	assert.NotNil(t, res.NB.Listeners)
	assert.NotNil(t, res.SB.Listeners)

	// A three-member NB cluster: a leader, a follower, and one unreachable
	// endpoint (nil client). Each member's "_Server" reports only its own row.
	cid := "aaaaaaaa-0000-0000-0000-000000000001"
	sidLeader := "bbbbbbbb-0000-0000-0000-000000000001"
	sidFollower := "bbbbbbbb-0000-0000-0000-000000000002"
	leaderIdx, followerIdx := 42, 41
	leader := testutil.SetupServerMonitorTestClient(t,
		serverdb.Database{Name: "_Server", Model: serverdb.DatabaseModelClustered},
		serverdb.Database{
			Name: "OVN_Northbound", Model: serverdb.DatabaseModelClustered,
			Connected: true, Leader: true, Cid: &cid, Sid: &sidLeader, Index: &leaderIdx,
		},
	)
	follower := testutil.SetupServerMonitorTestClient(t,
		serverdb.Database{
			Name: "OVN_Northbound", Model: serverdb.DatabaseModelClustered,
			Connected: true, Leader: false, Cid: &cid, Sid: &sidFollower, Index: &followerIdx,
		},
	)
	querier.NBServers = []ovndb.ServerMonitor{
		{Endpoint: "tcp:n1:6641", Client: leader},
		{Endpoint: "tcp:n2:6641", Client: follower},
		{Endpoint: "tcp:n3:6641", Client: nil}, // down at startup
	}

	res, err = querier.RaftHealth(context.Background())
	require.NoError(t, err)
	cl := res.NB.Cluster
	require.True(t, cl.Available)
	assert.Equal(t, "clustered", cl.Model)
	assert.Equal(t, cid, cl.ClusterID)
	assert.False(t, cl.SplitBrain)
	assert.Equal(t, 3, cl.Total)
	assert.Equal(t, 2, cl.Reachable)
	assert.True(t, cl.HasLeader)
	assert.Equal(t, sidLeader, cl.LeaderID)
	require.Len(t, cl.Members, 3)

	assert.Equal(t, "tcp:n1:6641", cl.Members[0].Endpoint)
	assert.True(t, cl.Members[0].Reachable)
	assert.True(t, cl.Members[0].Leader)
	assert.True(t, cl.Members[0].Connected)
	assert.Equal(t, sidLeader, cl.Members[0].ServerID)
	assert.Equal(t, 42, cl.Members[0].Index)

	assert.False(t, cl.Members[1].Leader)
	assert.Equal(t, sidFollower, cl.Members[1].ServerID)
	assert.Equal(t, 41, cl.Members[1].Index)

	assert.Equal(t, "tcp:n3:6641", cl.Members[2].Endpoint)
	assert.False(t, cl.Members[2].Reachable)

	// SB still has no _Server monitors wired in.
	assert.False(t, res.SB.Cluster.Available)
}
