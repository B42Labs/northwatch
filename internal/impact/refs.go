package impact

import (
	"reflect"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// NBRefs defines forward references from NB tables to their children.
// Keys are OVSDB table names; values list columns that reference other tables.
var NBRefs = map[string][]RefEdge{
	"Logical_Switch": {
		{Column: "ports", TargetTable: "Logical_Switch_Port", Type: RefStrong},
		{Column: "acls", TargetTable: "ACL", Type: RefStrong},
		{Column: "qos_rules", TargetTable: "QoS", Type: RefStrong},
		{Column: "forwarding_groups", TargetTable: "Forwarding_Group", Type: RefStrong},
		{Column: "load_balancer_group", TargetTable: "Load_Balancer_Group", Type: RefStrong},
		{Column: "load_balancer", TargetTable: "Load_Balancer", Type: RefWeak},
		{Column: "dns_records", TargetTable: "DNS", Type: RefWeak},
		{Column: "copp", TargetTable: "Copp", Type: RefWeak},
	},
	"Logical_Router": {
		{Column: "ports", TargetTable: "Logical_Router_Port", Type: RefStrong},
		{Column: "nat", TargetTable: "NAT", Type: RefStrong},
		{Column: "policies", TargetTable: "Logical_Router_Policy", Type: RefStrong},
		{Column: "static_routes", TargetTable: "Logical_Router_Static_Route", Type: RefStrong},
		{Column: "load_balancer_group", TargetTable: "Load_Balancer_Group", Type: RefStrong},
		{Column: "load_balancer", TargetTable: "Load_Balancer", Type: RefWeak},
		{Column: "copp", TargetTable: "Copp", Type: RefWeak},
	},
	"Logical_Switch_Port": {
		{Column: "ha_chassis_group", TargetTable: "HA_Chassis_Group", Type: RefStrong},
	},
	"Logical_Router_Port": {
		{Column: "gateway_chassis", TargetTable: "Gateway_Chassis", Type: RefStrong},
		{Column: "ha_chassis_group", TargetTable: "HA_Chassis_Group", Type: RefStrong},
	},
	"HA_Chassis_Group": {
		{Column: "ha_chassis", TargetTable: "HA_Chassis", Type: RefStrong},
	},
	"Port_Group": {
		{Column: "acls", TargetTable: "ACL", Type: RefStrong},
		{Column: "ports", TargetTable: "Logical_Switch_Port", Type: RefWeak},
	},
	"Load_Balancer": {
		{Column: "health_check", TargetTable: "Load_Balancer_Health_Check", Type: RefStrong},
	},
	"Load_Balancer_Group": {
		{Column: "load_balancer", TargetTable: "Load_Balancer", Type: RefWeak},
	},
}

// ReverseRefs maps a target table to the set of tables that reference it.
// When the target is deleted, these source entities lose their reference.
var ReverseRefs = map[string][]ReverseRefEdge{
	"Logical_Switch_Port": {
		{SourceTable: "Port_Group", Column: "ports"},
	},
	"Load_Balancer": {
		{SourceTable: "Logical_Switch", Column: "load_balancer"},
		{SourceTable: "Logical_Router", Column: "load_balancer"},
		{SourceTable: "Load_Balancer_Group", Column: "load_balancer"},
	},
	"DHCP_Options": {
		{SourceTable: "Logical_Switch_Port", Column: "dhcpv4_options"},
		{SourceTable: "Logical_Switch_Port", Column: "dhcpv6_options"},
	},
	"Address_Set": {
		{SourceTable: "NAT", Column: "allowed_ext_ips"},
		{SourceTable: "NAT", Column: "exempted_ext_ips"},
	},
	"DNS": {
		{SourceTable: "Logical_Switch", Column: "dns_records"},
	},
}

// SBCorrelations maps NB root entity tables to their SB Datapath_Binding correlation.
var SBCorrelations = map[string]SBCorrelationDef{
	"Logical_Switch": {DatapathKey: "logical-switch"},
	"Logical_Router": {DatapathKey: "logical-router"},
}

// NBModelTypes maps NB table names to their Go reflect.Type so the resolver
// can instantiate models for arbitrary tables.
var NBModelTypes = map[string]reflect.Type{
	"Logical_Switch":              reflect.TypeOf(nb.LogicalSwitch{}),
	"Logical_Switch_Port":         reflect.TypeOf(nb.LogicalSwitchPort{}),
	"Logical_Router":              reflect.TypeOf(nb.LogicalRouter{}),
	"Logical_Router_Port":         reflect.TypeOf(nb.LogicalRouterPort{}),
	"ACL":                         reflect.TypeOf(nb.ACL{}),
	"NAT":                         reflect.TypeOf(nb.NAT{}),
	"Address_Set":                 reflect.TypeOf(nb.AddressSet{}),
	"Port_Group":                  reflect.TypeOf(nb.PortGroup{}),
	"Load_Balancer":               reflect.TypeOf(nb.LoadBalancer{}),
	"Load_Balancer_Group":         reflect.TypeOf(nb.LoadBalancerGroup{}),
	"Load_Balancer_Health_Check":  reflect.TypeOf(nb.LoadBalancerHealthCheck{}),
	"Logical_Router_Static_Route": reflect.TypeOf(nb.LogicalRouterStaticRoute{}),
	"Logical_Router_Policy":       reflect.TypeOf(nb.LogicalRouterPolicy{}),
	"DHCP_Options":                reflect.TypeOf(nb.DHCPOptions{}),
	"DNS":                         reflect.TypeOf(nb.DNS{}),
	"QoS":                         reflect.TypeOf(nb.QoS{}),
	"Forwarding_Group":            reflect.TypeOf(nb.ForwardingGroup{}),
	"Gateway_Chassis":             reflect.TypeOf(nb.GatewayChassis{}),
	"HA_Chassis_Group":            reflect.TypeOf(nb.HAChassisGroup{}),
	"HA_Chassis":                  reflect.TypeOf(nb.HAChassis{}),
	"Static_MAC_Binding":          reflect.TypeOf(nb.StaticMACBinding{}),
	"Copp":                        reflect.TypeOf(nb.Copp{}),
}

// SBModelTypes maps SB table names to their Go reflect.Type.
var SBModelTypes = map[string]reflect.Type{
	"Datapath_Binding": reflect.TypeOf(sb.DatapathBinding{}),
	"Port_Binding":     reflect.TypeOf(sb.PortBinding{}),
}
