package testutil

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/ovsdb/vs"
	"github.com/go-logr/stdr"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/ovsdb/serverdb"
	"github.com/ovn-kubernetes/libovsdb/server"
	"github.com/stretchr/testify/require"
)

// SetupNBTestClient creates an in-memory NB OVSDB test server and returns a connected client.
func SetupNBTestClient(t *testing.T) client.Client {
	t.Helper()
	clientModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	schema := nb.Schema()
	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)
	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{schema.Name: clientModel}, &logger)
	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)
	sockPath := filepath.Join(t.TempDir(), "nb.sock")
	go func() { _ = ovsdbServer.Serve("unix", sockPath) }()
	require.Eventually(t, func() bool { return ovsdbServer.Ready() }, 5*time.Second, 10*time.Millisecond)
	t.Cleanup(func() { ovsdbServer.Close() })
	c, err := client.NewOVSDBClient(clientModel, client.WithEndpoint(fmt.Sprintf("unix:%s", sockPath)))
	require.NoError(t, err)
	require.NoError(t, c.Connect(context.Background()))
	_, err = c.MonitorAll(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	return c
}

// SetupSBTestClient creates an in-memory SB OVSDB test server and returns a connected client.
func SetupSBTestClient(t *testing.T) client.Client {
	t.Helper()
	clientModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)
	schema := sb.Schema()
	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)
	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{schema.Name: clientModel}, &logger)
	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)
	sockPath := filepath.Join(t.TempDir(), "sb.sock")
	go func() { _ = ovsdbServer.Serve("unix", sockPath) }()
	require.Eventually(t, func() bool { return ovsdbServer.Ready() }, 5*time.Second, 10*time.Millisecond)
	t.Cleanup(func() { ovsdbServer.Close() })
	c, err := client.NewOVSDBClient(clientModel, client.WithEndpoint(fmt.Sprintf("unix:%s", sockPath)))
	require.NoError(t, err)
	require.NoError(t, c.Connect(context.Background()))
	_, err = c.MonitorAll(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	return c
}

// SetupOVSTestServer creates an in-memory per-chassis Open_vSwitch OVSDB test
// server and returns its unix socket path (so a pool can dial "unix:<sock>")
// alongside a connected, monitoring client for seeding rows.
func SetupOVSTestServer(t *testing.T) (string, client.Client) {
	t.Helper()
	clientModel, err := vs.FullDatabaseModel()
	require.NoError(t, err)
	schema := vs.Schema()
	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)
	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{schema.Name: clientModel}, &logger)
	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)
	sockPath := filepath.Join(t.TempDir(), "ovs.sock")
	go func() { _ = ovsdbServer.Serve("unix", sockPath) }()
	require.Eventually(t, func() bool { return ovsdbServer.Ready() }, 5*time.Second, 10*time.Millisecond)
	t.Cleanup(func() { ovsdbServer.Close() })
	c, err := client.NewOVSDBClient(clientModel, client.WithEndpoint(fmt.Sprintf("unix:%s", sockPath)))
	require.NoError(t, err)
	require.NoError(t, c.Connect(context.Background()))
	_, err = c.MonitorAll(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	return sockPath, c
}

// InsertOVSBridgeWithInterface creates an Open_vSwitch root row referencing one
// Bridge → Port → Interface graph in a single transaction. Only Open_vSwitch is
// a root table, so the bridge, port and interface are non-root and must be
// inserted together with the referencing root or OVSDB referential integrity
// garbage-collects them immediately (mirroring InsertGatewayRouter). The
// interface carries the given statistics and link_state so a test can assert on
// the live telemetry fields.
func InsertOVSBridgeWithInterface(t *testing.T, c client.Client, bridgeName, ifaceName string, stats map[string]int, linkState string) {
	t.Helper()
	if stats == nil {
		stats = map[string]int{}
	}

	var allOps []ovsdb.Operation

	ifaceNamed := "iface_" + ifaceName
	ls := linkState
	iface := &vs.Interface{
		UUID:        ifaceNamed,
		Name:        ifaceName,
		Type:        "system",
		LinkState:   &ls,
		Statistics:  stats,
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
		Status:      map[string]string{},
	}
	ops, err := c.Create(iface)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	portNamed := "port_" + ifaceName
	port := &vs.Port{
		UUID:        portNamed,
		Name:        ifaceName,
		Interfaces:  []string{ifaceNamed},
		ExternalIDs: map[string]string{},
		OtherConfig: map[string]string{},
	}
	ops, err = c.Create(port)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	bridgeNamed := "bridge_" + bridgeName
	bridge := &vs.Bridge{
		UUID:         bridgeNamed,
		Name:         bridgeName,
		DatapathType: "system",
		Ports:        []string{portNamed},
		ExternalIDs:  map[string]string{},
		OtherConfig:  map[string]string{},
	}
	ops, err = c.Create(bridge)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	root := &vs.OpenvSwitch{
		Bridges:     []string{bridgeNamed},
		ExternalIDs: map[string]string{},
		OtherConfig: map[string]string{},
	}
	ops, err = c.Create(root)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var ifaces []vs.Interface
		if err := c.List(context.Background(), &ifaces); err != nil {
			return false
		}
		for _, i := range ifaces {
			if i.Name == ifaceName {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

// InsertOVSSSLRow creates an Open_vSwitch root row referencing one SSL row in a
// single transaction. SSL is a non-root table, so it must be inserted together
// with the referencing root (via Open_vSwitch.ssl) or OVSDB referential
// integrity garbage-collects it immediately (mirroring
// InsertOVSBridgeWithInterface). The row carries the given private key and certs
// so a test can assert that private_key is redacted while the certs are served.
func InsertOVSSSLRow(t *testing.T, c client.Client, privateKey, certificate, caCert string) {
	t.Helper()

	sslNamed := "ssl_row"
	ssl := &vs.SSL{
		UUID:        sslNamed,
		PrivateKey:  privateKey,
		Certificate: certificate,
		CaCert:      caCert,
		ExternalIDs: map[string]string{},
	}
	ops, err := c.Create(ssl)
	require.NoError(t, err)
	allOps := ops

	sslRef := sslNamed
	root := &vs.OpenvSwitch{
		SSL:         &sslRef,
		ExternalIDs: map[string]string{},
		OtherConfig: map[string]string{},
	}
	ops, err = c.Create(root)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var rows []vs.SSL
		if err := c.List(context.Background(), &rows); err != nil {
			return false
		}
		return len(rows) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

// SetupServerMonitorTestClient creates an in-memory "_Server" OVSDB test server
// seeded with the given Database rows (mirroring how a real ovsdb-server exposes
// Raft cluster state) and returns a connected, monitoring client.
func SetupServerMonitorTestClient(t *testing.T, rows ...serverdb.Database) client.Client {
	t.Helper()
	clientModel, err := serverdb.FullDatabaseModel()
	require.NoError(t, err)
	schema := serverdb.Schema()
	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	require.Empty(t, errs)
	logger := stdr.New(nil)
	db := inmemory.NewDatabase(map[string]model.ClientDBModel{schema.Name: clientModel}, &logger)
	ovsdbServer, err := server.NewOvsdbServer(db, &logger, dbModel)
	require.NoError(t, err)
	sockPath := filepath.Join(t.TempDir(), "server.sock")
	go func() { _ = ovsdbServer.Serve("unix", sockPath) }()
	require.Eventually(t, func() bool { return ovsdbServer.Ready() }, 5*time.Second, 10*time.Millisecond)
	t.Cleanup(func() { ovsdbServer.Close() })
	c, err := client.NewOVSDBClient(clientModel, client.WithEndpoint(fmt.Sprintf("unix:%s", sockPath)))
	require.NoError(t, err)
	require.NoError(t, c.Connect(context.Background()))
	_, err = c.MonitorAll(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	for i := range rows {
		row := rows[i]
		ops, err := c.Create(&row)
		require.NoError(t, err)
		reply, err := c.Transact(context.Background(), ops...)
		require.NoError(t, err)
		_, err = ovsdb.CheckOperationResults(reply, ops)
		require.NoError(t, err)
		uuid := reply[0].UUID.GoUUID
		require.Eventually(t, func() bool {
			return c.Get(context.Background(), &serverdb.Database{UUID: uuid}) == nil
		}, 2*time.Second, 10*time.Millisecond)
	}
	return c
}

// InsertNBGlobal inserts an NB_Global row with the given config generations.
func InsertNBGlobal(t *testing.T, c client.Client, nbCfg, sbCfg, hvCfg int) {
	t.Helper()
	g := &nb.NBGlobal{NbCfg: nbCfg, SbCfg: sbCfg, HvCfg: hvCfg, ExternalIDs: map[string]string{}, Options: map[string]string{}}
	ops, err := c.Create(g)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.NBGlobal{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
}

// InsertNBGlobalWithOptions inserts an NB_Global row carrying the given options
// (e.g. mac_binding_age_threshold) with zero config generations.
func InsertNBGlobalWithOptions(t *testing.T, c client.Client, options map[string]string) {
	t.Helper()
	if options == nil {
		options = map[string]string{}
	}
	g := &nb.NBGlobal{ExternalIDs: map[string]string{}, Options: options}
	ops, err := c.Create(g)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.NBGlobal{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
}

// InsertLogicalSwitch inserts a Logical_Switch row.
func InsertLogicalSwitch(t *testing.T, c client.Client, name string) string {
	t.Helper()
	ls := &nb.LogicalSwitch{Name: name, ExternalIDs: map[string]string{}}
	ops, err := c.Create(ls)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LogicalSwitch{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// InsertSBGlobal inserts an SB_Global row.
func InsertSBGlobal(t *testing.T, c client.Client, nbCfg int) {
	t.Helper()
	g := &sb.SBGlobal{NbCfg: nbCfg, ExternalIDs: map[string]string{}, Options: map[string]string{}}
	ops, err := c.Create(g)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.SBGlobal{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
}

// InsertChassis inserts a Chassis row with an associated Encap.
func InsertChassis(t *testing.T, c client.Client, name, hostname, ip string) string {
	t.Helper()
	namedEncapUUID := "encap_" + name
	encap := &sb.Encap{UUID: namedEncapUUID, Type: "geneve", IP: ip, ChassisName: name}
	encapOps, err := c.Create(encap)
	require.NoError(t, err)
	chassis := &sb.Chassis{
		Name: name, Hostname: hostname, Encaps: []string{namedEncapUUID},
		ExternalIDs: map[string]string{}, OtherConfig: map[string]string{},
	}
	chassisOps, err := c.Create(chassis)
	require.NoError(t, err)
	ops := append(encapOps, chassisOps...)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[1].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.Chassis{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

var tunnelKeyCounter atomic.Int64

// InsertDatapathBinding inserts a Datapath_Binding row with a unique tunnel key.
func InsertDatapathBinding(t *testing.T, c client.Client) string {
	t.Helper()
	dp := &sb.DatapathBinding{TunnelKey: int(tunnelKeyCounter.Add(1)), ExternalIDs: map[string]string{}}
	ops, err := c.Create(dp)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.DatapathBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// InsertPortBinding inserts a Port_Binding row.
func InsertPortBinding(t *testing.T, c client.Client, logicalPort, pbType string, chassisUUID *string) string {
	t.Helper()
	dpUUID := InsertDatapathBinding(t, c)
	pb := &sb.PortBinding{
		LogicalPort: logicalPort,
		Type:        pbType,
		Datapath:    dpUUID,
		Chassis:     chassisUUID,
		TunnelKey:   int(tunnelKeyCounter.Add(1)),
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	ops, err := c.Create(pb)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.PortBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// InsertBFD inserts a BFD row.
func InsertBFD(t *testing.T, c client.Client, chassisName, dstIP, logicalPort string, status sb.BFDStatus) string {
	t.Helper()
	bfd := &sb.BFD{
		ChassisName: chassisName,
		DstIP:       dstIP,
		LogicalPort: logicalPort,
		Status:      status,
		SrcPort:     49152,
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	ops, err := c.Create(bfd)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.BFD{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// InsertPortBindingWithUp inserts a Port_Binding row with the Up field set.
func InsertPortBindingWithUp(t *testing.T, c client.Client, logicalPort, pbType string, chassisUUID *string, up *bool) string {
	t.Helper()
	dpUUID := InsertDatapathBinding(t, c)
	pb := &sb.PortBinding{
		LogicalPort: logicalPort,
		Type:        pbType,
		Datapath:    dpUUID,
		Chassis:     chassisUUID,
		Up:          up,
		TunnelKey:   int(tunnelKeyCounter.Add(1)),
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	ops, err := c.Create(pb)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.PortBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// InsertChassisPrivate inserts a Chassis_Private row.
func InsertChassisPrivate(t *testing.T, c client.Client, name string, chassisUUID *string, nbCfg, nbCfgTimestamp int) string {
	t.Helper()
	cp := &sb.ChassisPrivate{
		Name:           name,
		Chassis:        chassisUUID,
		NbCfg:          nbCfg,
		NbCfgTimestamp: nbCfgTimestamp,
		ExternalIDs:    map[string]string{},
	}
	ops, err := c.Create(cp)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.ChassisPrivate{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// UpdateNBGlobal updates the NB_Global row's nb_cfg and nb_cfg_timestamp fields.
func UpdateNBGlobal(t *testing.T, c client.Client, nbCfg, nbCfgTimestamp int) {
	t.Helper()
	var globals []nb.NBGlobal
	require.NoError(t, c.List(context.Background(), &globals))
	require.NotEmpty(t, globals)
	g := &globals[0]
	g.NbCfg = nbCfg
	g.NbCfgTimestamp = nbCfgTimestamp
	ops, err := c.Where(g).Update(g, &g.NbCfg, &g.NbCfgTimestamp)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var gs []nb.NBGlobal
		if err := c.List(context.Background(), &gs); err != nil || len(gs) == 0 {
			return false
		}
		return gs[0].NbCfg == nbCfg
	}, 2*time.Second, 10*time.Millisecond)
}

// UpdateChassisPrivate updates a Chassis_Private row's nb_cfg and nb_cfg_timestamp.
func UpdateChassisPrivate(t *testing.T, c client.Client, name string, nbCfg, nbCfgTimestamp int) {
	t.Helper()
	var privates []sb.ChassisPrivate
	require.NoError(t, c.List(context.Background(), &privates))
	var target *sb.ChassisPrivate
	for i := range privates {
		if privates[i].Name == name {
			target = &privates[i]
			break
		}
	}
	require.NotNil(t, target, "Chassis_Private %q not found", name)
	target.NbCfg = nbCfg
	target.NbCfgTimestamp = nbCfgTimestamp
	ops, err := c.Where(target).Update(target, &target.NbCfg, &target.NbCfgTimestamp)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var ps []sb.ChassisPrivate
		if err := c.List(context.Background(), &ps); err != nil {
			return false
		}
		for _, p := range ps {
			if p.Name == name && p.NbCfg == nbCfg {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

// InsertChassisRedirectBinding inserts a Port_Binding of type "chassisredirect"
// (the cr-lrp port), optionally referencing an HA_Chassis_Group as its
// election source. chassisUUID is the actual active owner (may be nil).
func InsertChassisRedirectBinding(t *testing.T, c client.Client, logicalPort string, chassisUUID *string, haGroupUUID string) string {
	t.Helper()
	dpUUID := InsertDatapathBinding(t, c)
	pb := &sb.PortBinding{
		LogicalPort: logicalPort,
		Type:        "chassisredirect",
		Datapath:    dpUUID,
		Chassis:     chassisUUID,
		TunnelKey:   int(tunnelKeyCounter.Add(1)),
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	if haGroupUUID != "" {
		g := haGroupUUID
		pb.HaChassisGroup = &g
	}
	ops, err := c.Create(pb)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.PortBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// InsertGatewayRouter creates a Logical_Router with one Logical_Router_Port and
// optional dnat_and_snat NAT rows (one per fip) in a single transaction.
// Logical_Router_Port and NAT are non-root tables, so they must be created
// together with their referencing root (the Logical_Router) or OVSDB referential
// integrity garbage-collects them immediately.
func InsertGatewayRouter(t *testing.T, c client.Client, routerName, lrpName string, networks, fips []string) {
	t.Helper()
	if len(networks) == 0 {
		networks = []string{"0.0.0.0/0"}
	}

	var allOps []ovsdb.Operation

	lrpNamed := "lrp_" + lrpName
	lrp := &nb.LogicalRouterPort{
		UUID:          lrpNamed,
		Name:          lrpName,
		Networks:      networks,
		MAC:           "00:00:00:00:00:01",
		ExternalIDs:   map[string]string{},
		Options:       map[string]string{},
		Ipv6RaConfigs: map[string]string{},
		Status:        map[string]string{},
	}
	ops, err := c.Create(lrp)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	natNamed := make([]string, len(fips))
	for i, fip := range fips {
		natNamed[i] = fmt.Sprintf("nat_%s_%d", lrpName, i)
		nat := &nb.NAT{
			UUID:        natNamed[i],
			Type:        nb.NATType("dnat_and_snat"),
			ExternalIP:  fip,
			LogicalIP:   "192.168.0.10",
			ExternalIDs: map[string]string{},
			Options:     map[string]string{},
		}
		ops, err := c.Create(nat)
		require.NoError(t, err)
		allOps = append(allOps, ops...)
	}

	lr := &nb.LogicalRouter{
		Name:        routerName,
		Ports:       []string{lrpNamed},
		Nat:         natNamed,
		ExternalIDs: map[string]string{},
		Options:     map[string]string{},
	}
	ops, err = c.Create(lr)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var routers []nb.LogicalRouter
		if err := c.List(context.Background(), &routers); err != nil {
			return false
		}
		for _, r := range routers {
			if r.Name == routerName && len(r.Ports) == 1 {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

// StaticRouteSpec describes one Logical_Router_Static_Route for
// InsertRouterWithStaticRoutes.
type StaticRouteSpec struct {
	IPPrefix   string
	Nexthop    string
	OutputPort *string
}

// InsertRouterWithStaticRoutes creates a Logical_Router with one
// Logical_Router_Port and the given static routes in a single transaction. The
// router port and static routes are non-root tables, so they must be created
// together with their referencing root (the Logical_Router). Returns the
// router UUID.
func InsertRouterWithStaticRoutes(t *testing.T, c client.Client, routerName, lrpName string, networks []string, options map[string]string, routes []StaticRouteSpec) string {
	t.Helper()
	if len(networks) == 0 {
		networks = []string{"0.0.0.0/0"}
	}
	if options == nil {
		options = map[string]string{}
	}

	var allOps []ovsdb.Operation

	lrpNamed := "lrp_" + lrpName
	lrp := &nb.LogicalRouterPort{
		UUID:          lrpNamed,
		Name:          lrpName,
		Networks:      networks,
		MAC:           "00:00:00:00:00:01",
		ExternalIDs:   map[string]string{},
		Options:       map[string]string{},
		Ipv6RaConfigs: map[string]string{},
		Status:        map[string]string{},
	}
	ops, err := c.Create(lrp)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	routeNamed := make([]string, len(routes))
	for i, rs := range routes {
		routeNamed[i] = fmt.Sprintf("route_%s_%d", lrpName, i)
		r := &nb.LogicalRouterStaticRoute{
			UUID:        routeNamed[i],
			IPPrefix:    rs.IPPrefix,
			Nexthop:     rs.Nexthop,
			OutputPort:  rs.OutputPort,
			ExternalIDs: map[string]string{},
			Options:     map[string]string{},
		}
		ops, err := c.Create(r)
		require.NoError(t, err)
		allOps = append(allOps, ops...)
	}

	lr := &nb.LogicalRouter{
		Name:         routerName,
		Ports:        []string{lrpNamed},
		StaticRoutes: routeNamed,
		Options:      options,
		ExternalIDs:  map[string]string{},
	}
	ops, err = c.Create(lr)
	require.NoError(t, err)
	allOps = append(allOps, ops...)

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)
	routerUUID := reply[len(reply)-1].UUID.GoUUID

	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &nb.LogicalRouter{UUID: routerUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return routerUUID
}

// InsertMACBinding inserts a dynamic SB MAC_Binding row (an entry in a logical
// router's ARP/ND cache). timestampMillis is the libovsdb timestamp column in
// epoch milliseconds; pass 0 to leave it unset.
func InsertMACBinding(t *testing.T, c client.Client, logicalPort, ip, mac string, timestampMillis int) string {
	t.Helper()
	dpUUID := InsertDatapathBinding(t, c)
	m := &sb.MACBinding{
		Datapath:    dpUUID,
		LogicalPort: logicalPort,
		IP:          ip,
		MAC:         mac,
		Timestamp:   timestampMillis,
	}
	ops, err := c.Create(m)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.MACBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// InsertStaticMACBinding inserts an SB Static_MAC_Binding row pinning a
// next-hop MAC for a (logical_port, ip) pair.
func InsertStaticMACBinding(t *testing.T, c client.Client, logicalPort, ip, mac string, override bool) string {
	t.Helper()
	dpUUID := InsertDatapathBinding(t, c)
	s := &sb.StaticMACBinding{
		Datapath:           dpUUID,
		LogicalPort:        logicalPort,
		IP:                 ip,
		MAC:                mac,
		OverrideDynamicMAC: override,
	}
	ops, err := c.Create(s)
	require.NoError(t, err)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)
	uuid := reply[0].UUID.GoUUID
	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.StaticMACBinding{UUID: uuid}) == nil
	}, 2*time.Second, 10*time.Millisecond)
	return uuid
}

// HAChassisEntry describes a single HA_Chassis member for InsertHAChassisGroup.
type HAChassisEntry struct {
	ChassisUUID string
	Priority    int
}

// InsertHAChassisGroupResult holds the UUIDs returned from InsertHAChassisGroup.
type InsertHAChassisGroupResult struct {
	GroupUUID      string
	HaChassisUUIDs []string
}

// InsertHAChassisGroup inserts an HA_Chassis_Group row along with its HA_Chassis members
// in a single transaction to satisfy strong reference constraints.
func InsertHAChassisGroup(t *testing.T, c client.Client, name string, members []HAChassisEntry) InsertHAChassisGroupResult {
	t.Helper()

	var allOps []ovsdb.Operation
	namedUUIDs := make([]string, len(members))

	for i, m := range members {
		namedUUID := fmt.Sprintf("hac_%s_%d", name, i)
		namedUUIDs[i] = namedUUID
		chassisRef := m.ChassisUUID
		hac := &sb.HAChassis{
			UUID:        namedUUID,
			Chassis:     &chassisRef,
			Priority:    m.Priority,
			ExternalIDs: map[string]string{},
		}
		ops, err := c.Create(hac)
		require.NoError(t, err)
		allOps = append(allOps, ops...)
	}

	group := &sb.HAChassisGroup{
		Name:        name,
		HaChassis:   namedUUIDs,
		ExternalIDs: map[string]string{},
	}
	groupOps, err := c.Create(group)
	require.NoError(t, err)
	allOps = append(allOps, groupOps...)

	reply, err := c.Transact(context.Background(), allOps...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, allOps)
	require.NoError(t, err)

	// Collect real UUIDs: first len(members) replies are HAChassis, last is the group
	result := InsertHAChassisGroupResult{
		GroupUUID:      reply[len(members)].UUID.GoUUID,
		HaChassisUUIDs: make([]string, len(members)),
	}
	for i := 0; i < len(members); i++ {
		result.HaChassisUUIDs[i] = reply[i].UUID.GoUUID
	}

	require.Eventually(t, func() bool {
		return c.Get(context.Background(), &sb.HAChassisGroup{UUID: result.GroupUUID}) == nil
	}, 2*time.Second, 10*time.Millisecond)

	return result
}
