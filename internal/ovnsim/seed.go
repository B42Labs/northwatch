package ovnsim

import (
	"context"
	"fmt"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// Options controls the size of the baseline topology Seed creates.
type Options struct {
	Switches       int      // number of tenant logical switches
	Routers        int      // number of logical routers (switches are spread across them)
	PortsPerSwitch int      // VIF ports per switch
	Chassis        []string // chassis names used for Gateway_Chassis / HA_Chassis_Group
}

// DefaultOptions returns a small but representative baseline.
func DefaultOptions() Options {
	return Options{
		Switches:       6,
		Routers:        3,
		PortsPerSwitch: 5,
		Chassis:        []string{"chassis-1", "chassis-2", "chassis-3"},
	}
}

func (o Options) withDefaults() Options {
	if o.Switches <= 0 {
		o.Switches = 1
	}
	if o.Routers <= 0 {
		o.Routers = 1
	}
	if o.PortsPerSwitch < 0 {
		o.PortsPerSwitch = 0
	}
	return o
}

// SeedResult records how many rows Seed created, per table.
type SeedResult struct {
	Created map[string]int
}

func (r *SeedResult) add(table string, n int) { r.Created[table] += n }

// Total returns the sum of all created rows.
func (r *SeedResult) Total() int {
	n := 0
	for _, v := range r.Created {
		n += v
	}
	return n
}

// Naming helpers — every name carries NamePrefix so simulator objects are easy
// to spot and the continuous simulator can recognise its own rows.
func switchName(s int) string           { return fmt.Sprintf("%sls-%03d", NamePrefix, s) }
func routerName(r int) string           { return fmt.Sprintf("%slr-%03d", NamePrefix, r) }
func lrpName(r, s int) string           { return fmt.Sprintf("%slrp-%03d-%03d", NamePrefix, r, s) }
func switchRouterPortName(s int) string { return fmt.Sprintf("%sls-%03d-lr", NamePrefix, s) }
func vifName(s, p int) string           { return fmt.Sprintf("%sls-%03d-vif-%03d", NamePrefix, s, p) }
func haGroupName(r int) string          { return fmt.Sprintf("%shagrp-lr-%03d", NamePrefix, r) }

// octet maps a 1-based switch index to a stable second IPv4 octet in 10.X.0.0/24.
func octet(s int) int { return ((s - 1) % 200) + 10 }

// routerForSwitch spreads switches across routers round-robin (1-based).
func routerForSwitch(s, routers int) int { return ((s - 1) % routers) + 1 }

// Seed creates an idempotent baseline OVN topology: tenant switches with VIF
// ports and DHCP, routers wired to those switches with NAT/routes/policies,
// plus security (port group + ACLs + address sets) and services (load
// balancers, meters, DNS, etc.). Re-running it only creates what is missing.
func Seed(ctx context.Context, c client.Client, opts Options) (*SeedResult, error) {
	opts = opts.withDefaults()
	res := &SeedResult{Created: map[string]int{}}

	switchNames, err := nameSet(ctx, c, func(s *nb.LogicalSwitch) string { return s.Name })
	if err != nil {
		return nil, err
	}
	routerNames, err := nameSet(ctx, c, func(r *nb.LogicalRouter) string { return r.Name })
	if err != nil {
		return nil, err
	}

	// Routers first so the switch's router-type port can reference the LRP by
	// name (the link is expressed via options:router-port).
	for r := 1; r <= opts.Routers; r++ {
		if _, ok := routerNames[routerName(r)]; ok {
			continue
		}
		if err := seedRouter(ctx, c, opts, r, res); err != nil {
			return nil, fmt.Errorf("seeding %s: %w", routerName(r), err)
		}
	}
	for s := 1; s <= opts.Switches; s++ {
		if _, ok := switchNames[switchName(s)]; ok {
			continue
		}
		if err := seedSwitch(ctx, c, opts, s, res); err != nil {
			return nil, fmt.Errorf("seeding %s: %w", switchName(s), err)
		}
	}

	if err := seedSecurity(ctx, c, res); err != nil {
		return nil, fmt.Errorf("seeding security: %w", err)
	}
	if err := seedServices(ctx, c, opts, res); err != nil {
		return nil, fmt.Errorf("seeding services: %w", err)
	}

	return res, nil
}

// seedRouter creates a logical router, its per-switch router ports (with a
// gateway chassis), an SNAT per attached subnet, a default route and a policy.
func seedRouter(ctx context.Context, c client.Client, opts Options, r int, res *SeedResult) error {
	t := newTxn(c)
	var ports, nats []string

	// Two redundancy mechanisms are demoed across the routers: even-numbered
	// routers point their gateway ports at a single HA_Chassis_Group (the modern
	// mechanism), odd-numbered routers use a per-port Gateway_Chassis. The HA
	// group's member priorities are what `ovnsim run` later shuffles to simulate
	// gateway failover.
	useHA := len(opts.Chassis) > 0 && r%2 == 0
	var haGroupUUID string
	if useHA {
		haGroupUUID = t.namedUUID()
		members := make([]string, 0, len(opts.Chassis))
		for i, ch := range opts.Chassis {
			m := t.namedUUID()
			t.add(&nb.HAChassis{
				UUID:        m,
				ChassisName: ch,
				Priority:    100 - i*10,
				ExternalIDs: ownedIDs("ha-chassis"),
			})
			members = append(members, m)
			res.add("HA_Chassis", 1)
		}
		t.add(&nb.HAChassisGroup{
			UUID:        haGroupUUID,
			Name:        haGroupName(r),
			HaChassis:   members,
			ExternalIDs: ownedIDs("ha-chassis-group"),
		})
		res.add("HA_Chassis_Group", 1)
	}

	// A router has exactly ONE distributed gateway port (the first port created),
	// matching real OVN topologies. Only that port carries the redundancy config
	// (HA group / gateway chassis), so ovn-northd realizes a single
	// chassisredirect (cr-) port per router in SB — which is what the gateway / HA
	// failover view is built from. The remaining tenant ports are plain patch
	// ports. (Making every tenant port a distributed gateway port — the previous
	// behaviour — is abnormal and kept the chassisredirect ports from appearing.)
	gwAssigned := false
	for s := 1; s <= opts.Switches; s++ {
		if routerForSwitch(s, opts.Routers) != r {
			continue
		}
		o := octet(s)

		lrpUUID := t.namedUUID()
		lrp := &nb.LogicalRouterPort{
			UUID:        lrpUUID,
			Name:        lrpName(r, s),
			MAC:         mac(0x100000 + r*256 + s),
			Networks:    []string{fmt.Sprintf("10.%d.0.1/24", o)},
			ExternalIDs: ownedIDs("router-port"),
		}
		if !gwAssigned {
			switch {
			case useHA:
				lrp.HaChassisGroup = ptr(haGroupUUID)
				gwAssigned = true
			case len(opts.Chassis) > 0:
				ch := opts.Chassis[0]
				gcUUID := t.namedUUID()
				t.add(&nb.GatewayChassis{
					UUID:        gcUUID,
					Name:        fmt.Sprintf("%s-gc", lrp.Name),
					ChassisName: ch,
					Priority:    30,
					ExternalIDs: ownedIDs("gateway-chassis"),
				})
				lrp.GatewayChassis = []string{gcUUID}
				res.add("Gateway_Chassis", 1)
				gwAssigned = true
			}
		}
		t.add(lrp)
		ports = append(ports, lrpUUID)
		res.add("Logical_Router_Port", 1)

		natUUID := t.namedUUID()
		t.add(&nb.NAT{
			UUID:        natUUID,
			Type:        nb.NATTypeSNAT,
			ExternalIP:  fmt.Sprintf("192.0.2.%d", r),
			LogicalIP:   fmt.Sprintf("10.%d.0.0/24", o),
			ExternalIDs: ownedIDs("nat"),
		})
		nats = append(nats, natUUID)
		res.add("NAT", 1)
	}

	routeUUID := t.namedUUID()
	t.add(&nb.LogicalRouterStaticRoute{
		UUID:        routeUUID,
		IPPrefix:    "0.0.0.0/0",
		Nexthop:     "10.255.255.254",
		ExternalIDs: ownedIDs("route"),
	})
	res.add("Logical_Router_Static_Route", 1)

	policyUUID := t.namedUUID()
	t.add(&nb.LogicalRouterPolicy{
		UUID:        policyUUID,
		Priority:    100,
		Match:       "ip4.dst == 10.0.0.0/8",
		Action:      nb.LogicalRouterPolicyActionAllow,
		ExternalIDs: ownedIDs("policy"),
	})
	res.add("Logical_Router_Policy", 1)

	t.add(&nb.LogicalRouter{
		UUID:         t.namedUUID(),
		Name:         routerName(r),
		Ports:        ports,
		Nat:          nats,
		StaticRoutes: []string{routeUUID},
		Policies:     []string{policyUUID},
		Options:      map[string]string{"chassis": ""},
		ExternalIDs:  ownedIDs("router"),
	})
	res.add("Logical_Router", 1)

	return t.commit(ctx)
}

// seedSwitch creates a tenant switch with DHCP options, VIF ports, a router
// uplink port, an ACL and a QoS rule.
func seedSwitch(ctx context.Context, c client.Client, opts Options, s int, res *SeedResult) error {
	t := newTxn(c)
	o := octet(s)
	subnet := fmt.Sprintf("10.%d.0.0/24", o)

	dhcpUUID := t.namedUUID()
	t.add(&nb.DHCPOptions{
		UUID: dhcpUUID,
		Cidr: subnet,
		Options: map[string]string{
			"server_id":  fmt.Sprintf("10.%d.0.1", o),
			"server_mac": mac(0x200000 + s),
			"lease_time": "3600",
			"router":     fmt.Sprintf("10.%d.0.1", o),
		},
		ExternalIDs: ownedIDs("dhcp"),
	})
	res.add("DHCP_Options", 1)

	var ports []string
	for p := 1; p <= opts.PortsPerSwitch; p++ {
		m := mac(0x300000 + s*256 + p)
		ip := fmt.Sprintf("10.%d.0.%d", o, p+9)
		lspUUID := t.namedUUID()
		t.add(&nb.LogicalSwitchPort{
			UUID:          lspUUID,
			Name:          vifName(s, p),
			Addresses:     []string{fmt.Sprintf("%s %s", m, ip)},
			PortSecurity:  []string{fmt.Sprintf("%s %s", m, ip)},
			Dhcpv4Options: ptr(dhcpUUID),
			Enabled:       ptr(true),
			ExternalIDs:   ownedIDs("vif"),
		})
		ports = append(ports, lspUUID)
		res.add("Logical_Switch_Port", 1)
	}

	// Router uplink port — the link to the LR is expressed by name.
	r := routerForSwitch(s, opts.Routers)
	uplinkUUID := t.namedUUID()
	t.add(&nb.LogicalSwitchPort{
		UUID:        uplinkUUID,
		Name:        switchRouterPortName(s),
		Type:        "router",
		Addresses:   []string{"router"},
		Options:     map[string]string{"router-port": lrpName(r, s)},
		ExternalIDs: ownedIDs("router-link"),
	})
	ports = append(ports, uplinkUUID)
	res.add("Logical_Switch_Port", 1)

	aclUUID := t.namedUUID()
	t.add(&nb.ACL{
		UUID:        aclUUID,
		Direction:   nb.ACLDirectionToLport,
		Priority:    1000,
		Match:       fmt.Sprintf("ip4.src == %s", subnet),
		Action:      nb.ACLActionAllowRelated,
		ExternalIDs: ownedIDs("acl"),
	})
	res.add("ACL", 1)

	qosUUID := t.namedUUID()
	t.add(&nb.QoS{
		UUID:        qosUUID,
		Direction:   nb.QoSDirectionFromLport,
		Priority:    100,
		Match:       fmt.Sprintf("ip4.src == %s", subnet),
		Action:      map[string]int{"dscp": 32},
		ExternalIDs: ownedIDs("qos"),
	})
	res.add("QoS", 1)

	t.add(&nb.LogicalSwitch{
		UUID:        t.namedUUID(),
		Name:        switchName(s),
		Ports:       ports,
		ACLs:        []string{aclUUID},
		QOSRules:    []string{qosUUID},
		OtherConfig: map[string]string{"subnet": subnet},
		ExternalIDs: ownedIDs("switch"),
	})
	res.add("Logical_Switch", 1)

	return t.commit(ctx)
}

// seedSecurity creates a web port group with ACLs and a couple of address sets.
func seedSecurity(ctx context.Context, c client.Client, res *SeedResult) error {
	pgNames, err := nameSet(ctx, c, func(p *nb.PortGroup) string { return p.Name })
	if err != nil {
		return err
	}
	if _, ok := pgNames["nw-pg-web"]; !ok {
		t := newTxn(c)
		acl1 := t.namedUUID()
		t.add(&nb.ACL{
			UUID:        acl1,
			Name:        ptr("nw-allow-http"),
			Direction:   nb.ACLDirectionToLport,
			Priority:    1100,
			Match:       "outport == @nw-pg-web && tcp.dst == 80",
			Action:      nb.ACLActionAllowRelated,
			ExternalIDs: ownedIDs("acl"),
		})
		acl2 := t.namedUUID()
		t.add(&nb.ACL{
			UUID:        acl2,
			Name:        ptr("nw-default-drop"),
			Direction:   nb.ACLDirectionToLport,
			Priority:    1000,
			Match:       "outport == @nw-pg-web && ip",
			Action:      nb.ACLActionDrop,
			ExternalIDs: ownedIDs("acl"),
		})
		t.add(&nb.PortGroup{
			UUID:        t.namedUUID(),
			Name:        "nw-pg-web",
			ACLs:        []string{acl1, acl2},
			ExternalIDs: ownedIDs("port-group"),
		})
		if err := t.commit(ctx); err != nil {
			return err
		}
		res.add("ACL", 2)
		res.add("Port_Group", 1)
	}

	asNames, err := nameSet(ctx, c, func(a *nb.AddressSet) string { return a.Name })
	if err != nil {
		return err
	}
	addrSets := map[string][]string{
		"nw-as-web": {"10.10.0.10", "10.10.0.11"},
		"nw-as-db":  {"10.11.0.20", "10.11.0.21"},
	}
	for name, addrs := range addrSets {
		if _, ok := asNames[name]; ok {
			continue
		}
		t := newTxn(c)
		t.add(&nb.AddressSet{Name: name, Addresses: addrs, ExternalIDs: ownedIDs("address-set")})
		if err := t.commit(ctx); err != nil {
			return err
		}
		res.add("Address_Set", 1)
	}
	return nil
}

// seedServices creates load balancers (+ group + health check), a meter, a DNS
// record and a Copp profile. Each block is gated on whether a simulator-owned
// row of that kind already exists.
func seedServices(ctx context.Context, c client.Client, opts Options, res *SeedResult) error {
	lbNames, err := nameSet(ctx, c, func(l *nb.LoadBalancer) string { return l.Name })
	if err != nil {
		return err
	}
	if _, ok := lbNames["nw-lb-001"]; !ok {
		t := newTxn(c)
		var lbs []string
		for i := 1; i <= 2; i++ {
			vip := fmt.Sprintf("192.0.2.%d:80", 100+i)
			hcUUID := t.namedUUID()
			t.add(&nb.LoadBalancerHealthCheck{
				UUID:        hcUUID,
				Vip:         vip,
				Options:     map[string]string{"interval": "5", "timeout": "20"},
				ExternalIDs: ownedIDs("lb-health-check"),
			})
			lbUUID := t.namedUUID()
			t.add(&nb.LoadBalancer{
				UUID:        lbUUID,
				Name:        fmt.Sprintf("nw-lb-%03d", i),
				Protocol:    ptr(nb.LoadBalancerProtocolTCP),
				Vips:        map[string]string{vip: "10.10.0.10:8080,10.10.0.11:8080"},
				HealthCheck: []string{hcUUID},
				ExternalIDs: ownedIDs("load-balancer"),
			})
			lbs = append(lbs, lbUUID)
			res.add("Load_Balancer", 1)
			res.add("Load_Balancer_Health_Check", 1)
		}
		t.add(&nb.LoadBalancerGroup{UUID: t.namedUUID(), Name: "nw-lb-group", LoadBalancer: lbs})
		if err := t.commit(ctx); err != nil {
			return err
		}
		res.add("Load_Balancer_Group", 1)
	}

	meterNames, err := nameSet(ctx, c, func(m *nb.Meter) string { return m.Name })
	if err != nil {
		return err
	}
	if _, ok := meterNames["nw-meter-1"]; !ok {
		t := newTxn(c)
		bandUUID := t.namedUUID()
		t.add(&nb.MeterBand{
			UUID:        bandUUID,
			Action:      nb.MeterBandActionDrop,
			Rate:        1000,
			BurstSize:   100,
			ExternalIDs: ownedIDs("meter-band"),
		})
		t.add(&nb.Meter{
			UUID:        t.namedUUID(),
			Name:        "nw-meter-1",
			Unit:        nb.MeterUnitKbps,
			Bands:       []string{bandUUID},
			ExternalIDs: ownedIDs("meter"),
		})
		if err := t.commit(ctx); err != nil {
			return err
		}
		res.add("Meter", 1)
		res.add("Meter_Band", 1)
	}

	hasDNS, err := hasOwned(ctx, c, func(d *nb.DNS) map[string]string { return d.ExternalIDs })
	if err != nil {
		return err
	}
	if !hasDNS {
		t := newTxn(c)
		t.add(&nb.DNS{
			Records:     map[string]string{"vm1.nw.local": "10.10.0.10", "vm2.nw.local": "10.10.0.11"},
			ExternalIDs: ownedIDs("dns"),
		})
		if err := t.commit(ctx); err != nil {
			return err
		}
		res.add("DNS", 1)
	}

	coppNames, err := nameSet(ctx, c, func(p *nb.Copp) string { return p.Name })
	if err != nil {
		return err
	}
	if _, ok := coppNames["nw-copp"]; !ok {
		t := newTxn(c)
		t.add(&nb.Copp{
			Name:        "nw-copp",
			Meters:      map[string]string{"arp": "nw-meter-1"},
			ExternalIDs: ownedIDs("copp"),
		})
		if err := t.commit(ctx); err != nil {
			return err
		}
		res.add("Copp", 1)
	}

	return nil
}
