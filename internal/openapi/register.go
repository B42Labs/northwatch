package openapi

import (
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// BuildSpec constructs the complete OpenAPI document for the Northwatch API.
// It mirrors the route registrations in cmd/northwatch/main.go.
func BuildSpec() Document {
	b := NewBuilder()

	registerHealth(b)
	registerCapabilities(b)
	registerNB(b)
	registerSB(b)
	registerCorrelated(b)
	registerSearch(b)
	registerTopology(b)
	registerFlows(b)
	registerDebug(b)
	registerHistory(b)
	registerAlerts(b)
	registerTelemetry(b)
	registerWrite(b)
	registerWebSocket(b)

	return b.Build()
}

func registerHealth(b *Builder) {
	b.AddOperation("/healthz", "get", &Operation{
		OperationID: "healthz",
		Summary:     "Liveness probe",
		Tags:        []string{"Health"},
		Responses:   jsonOK("Health status"),
	})
	b.AddOperation("/readyz", "get", &Operation{
		OperationID: "readyz",
		Summary:     "Readiness probe",
		Tags:        []string{"Health"},
		Responses:   jsonOK("Readiness status"),
	})
}

func registerCapabilities(b *Builder) {
	b.AddOperation("/api/v1/capabilities", "get", &Operation{
		OperationID: "getCapabilities",
		Summary:     "List enabled capabilities",
		Tags:        []string{"System"},
		Responses:   jsonOK("Capabilities list"),
	})
}

func registerNB(b *Builder) {
	// Tier 1
	AddTableEndpoints(b, nb.LogicalSwitch{}, "/api/v1/nb/logical-switches", "Northbound", "LogicalSwitch")
	AddTableEndpoints(b, nb.LogicalSwitchPort{}, "/api/v1/nb/logical-switch-ports", "Northbound", "LogicalSwitchPort")
	AddTableEndpoints(b, nb.LogicalRouter{}, "/api/v1/nb/logical-routers", "Northbound", "LogicalRouter")
	AddTableEndpoints(b, nb.LogicalRouterPort{}, "/api/v1/nb/logical-router-ports", "Northbound", "LogicalRouterPort")

	// Tier 2
	AddTableEndpoints(b, nb.ACL{}, "/api/v1/nb/acls", "Northbound", "ACL")
	AddTableEndpoints(b, nb.NAT{}, "/api/v1/nb/nats", "Northbound", "NAT")
	AddTableEndpoints(b, nb.AddressSet{}, "/api/v1/nb/address-sets", "Northbound", "NB_AddressSet")
	AddTableEndpoints(b, nb.PortGroup{}, "/api/v1/nb/port-groups", "Northbound", "NB_PortGroup")
	AddTableEndpoints(b, nb.LoadBalancer{}, "/api/v1/nb/load-balancers", "Northbound", "NB_LoadBalancer")
	AddTableEndpoints(b, nb.LoadBalancerGroup{}, "/api/v1/nb/load-balancer-groups", "Northbound", "LoadBalancerGroup")
	AddTableEndpoints(b, nb.LogicalRouterPolicy{}, "/api/v1/nb/logical-router-policies", "Northbound", "LogicalRouterPolicy")
	AddTableEndpoints(b, nb.LogicalRouterStaticRoute{}, "/api/v1/nb/logical-router-static-routes", "Northbound", "LogicalRouterStaticRoute")
	AddTableEndpoints(b, nb.DHCPOptions{}, "/api/v1/nb/dhcp-options", "Northbound", "DHCPOptions")

	// Tier 3
	AddTableEndpoints(b, nb.NBGlobal{}, "/api/v1/nb/nb-global", "Northbound", "NBGlobal")
	AddTableEndpoints(b, nb.Connection{}, "/api/v1/nb/connections", "Northbound", "NB_Connection")
	AddTableEndpoints(b, nb.DNS{}, "/api/v1/nb/dns", "Northbound", "NB_DNS")
	AddTableEndpoints(b, nb.GatewayChassis{}, "/api/v1/nb/gateway-chassis", "Northbound", "NB_GatewayChassis")
	AddTableEndpoints(b, nb.HAChassisGroup{}, "/api/v1/nb/ha-chassis-groups", "Northbound", "NB_HAChassisGroup")
	AddTableEndpoints(b, nb.Meter{}, "/api/v1/nb/meters", "Northbound", "NB_Meter")
	AddTableEndpoints(b, nb.QoS{}, "/api/v1/nb/qos", "Northbound", "QoS")
	AddTableEndpoints(b, nb.BFD{}, "/api/v1/nb/bfd", "Northbound", "NB_BFD")
	AddTableEndpoints(b, nb.Copp{}, "/api/v1/nb/copp", "Northbound", "Copp")
	AddTableEndpoints(b, nb.Mirror{}, "/api/v1/nb/mirrors", "Northbound", "NB_Mirror")
	AddTableEndpoints(b, nb.ForwardingGroup{}, "/api/v1/nb/forwarding-groups", "Northbound", "ForwardingGroup")
	AddTableEndpoints(b, nb.StaticMACBinding{}, "/api/v1/nb/static-mac-bindings", "Northbound", "NB_StaticMACBinding")
	AddTableEndpoints(b, nb.LoadBalancerHealthCheck{}, "/api/v1/nb/load-balancer-health-checks", "Northbound", "LoadBalancerHealthCheck")
}

func registerSB(b *Builder) {
	// Tier 1
	AddTableEndpoints(b, sb.Chassis{}, "/api/v1/sb/chassis", "Southbound", "Chassis")
	AddTableEndpoints(b, sb.PortBinding{}, "/api/v1/sb/port-bindings", "Southbound", "PortBinding")
	AddTableEndpoints(b, sb.DatapathBinding{}, "/api/v1/sb/datapath-bindings", "Southbound", "DatapathBinding")

	// Logical flows — special: has filtering via query params
	b.AddOperation("/api/v1/sb/logical-flows", "get", &Operation{
		OperationID: "listLogicalFlows",
		Summary:     "List logical flows with optional filtering",
		Tags:        []string{"Southbound"},
		Parameters: []Parameter{
			queryParam("datapath", "Filter by datapath UUID"),
			queryParam("pipeline", "Filter by pipeline (ingress/egress)"),
			queryParam("table_id", "Filter by table ID"),
			queryParam("match", "Filter by match expression substring"),
		},
		Responses: jsonOK("Array of LogicalFlow"),
	})
	b.AddOperation("/api/v1/sb/logical-flows/{uuid}", "get", &Operation{
		OperationID: "getLogicalFlow",
		Summary:     "Get logical flow by UUID",
		Tags:        []string{"Southbound"},
		Parameters:  []Parameter{pathParam("uuid")},
		Responses:   jsonOK("Single LogicalFlow"),
	})

	// Tier 2
	AddTableEndpoints(b, sb.Encap{}, "/api/v1/sb/encaps", "Southbound", "Encap")
	AddTableEndpoints(b, sb.MACBinding{}, "/api/v1/sb/mac-bindings", "Southbound", "MACBinding")
	AddTableEndpoints(b, sb.FDB{}, "/api/v1/sb/fdb", "Southbound", "FDB")
	AddTableEndpoints(b, sb.MulticastGroup{}, "/api/v1/sb/multicast-groups", "Southbound", "MulticastGroup")
	AddTableEndpoints(b, sb.AddressSet{}, "/api/v1/sb/address-sets", "Southbound", "SB_AddressSet")
	AddTableEndpoints(b, sb.PortGroup{}, "/api/v1/sb/port-groups", "Southbound", "SB_PortGroup")
	AddTableEndpoints(b, sb.LoadBalancer{}, "/api/v1/sb/load-balancers", "Southbound", "SB_LoadBalancer")
	AddTableEndpoints(b, sb.DNS{}, "/api/v1/sb/dns", "Southbound", "SB_DNS")

	// Tier 3
	AddTableEndpoints(b, sb.SBGlobal{}, "/api/v1/sb/sb-global", "Southbound", "SBGlobal")
	AddTableEndpoints(b, sb.Connection{}, "/api/v1/sb/connections", "Southbound", "SB_Connection")
	AddTableEndpoints(b, sb.GatewayChassis{}, "/api/v1/sb/gateway-chassis", "Southbound", "SB_GatewayChassis")
	AddTableEndpoints(b, sb.HAChassisGroup{}, "/api/v1/sb/ha-chassis-groups", "Southbound", "SB_HAChassisGroup")
	AddTableEndpoints(b, sb.HAChassis{}, "/api/v1/sb/ha-chassis", "Southbound", "HAChassis")
	AddTableEndpoints(b, sb.IPMulticast{}, "/api/v1/sb/ip-multicast", "Southbound", "IPMulticast")
	AddTableEndpoints(b, sb.IGMPGroup{}, "/api/v1/sb/igmp-groups", "Southbound", "IGMPGroup")
	AddTableEndpoints(b, sb.ServiceMonitor{}, "/api/v1/sb/service-monitors", "Southbound", "ServiceMonitor")
	AddTableEndpoints(b, sb.BFD{}, "/api/v1/sb/bfd", "Southbound", "SB_BFD")
	AddTableEndpoints(b, sb.Meter{}, "/api/v1/sb/meters", "Southbound", "SB_Meter")
	AddTableEndpoints(b, sb.Mirror{}, "/api/v1/sb/mirrors", "Southbound", "SB_Mirror")
	AddTableEndpoints(b, sb.ChassisPrivate{}, "/api/v1/sb/chassis-private", "Southbound", "ChassisPrivate")
	AddTableEndpoints(b, sb.ControllerEvent{}, "/api/v1/sb/controller-events", "Southbound", "ControllerEvent")
	AddTableEndpoints(b, sb.StaticMACBinding{}, "/api/v1/sb/static-mac-bindings", "Southbound", "SB_StaticMACBinding")
	AddTableEndpoints(b, sb.LogicalDPGroup{}, "/api/v1/sb/logical-dp-groups", "Southbound", "LogicalDPGroup")
	AddTableEndpoints(b, sb.RBACRole{}, "/api/v1/sb/rbac-roles", "Southbound", "RBACRole")
	AddTableEndpoints(b, sb.RBACPermission{}, "/api/v1/sb/rbac-permissions", "Southbound", "RBACPermission")
}

func registerCorrelated(b *Builder) {
	tag := "Correlated"
	b.AddOperation("/api/v1/correlated/logical-switches", "get", &Operation{
		OperationID: "listCorrelatedSwitches", Summary: "List correlated logical switches", Tags: []string{tag},
		Responses: jsonOK("Array of SwitchCorrelated"),
	})
	b.AddOperation("/api/v1/correlated/logical-switches/{uuid}", "get", &Operation{
		OperationID: "getCorrelatedSwitch", Summary: "Get correlated logical switch by UUID", Tags: []string{tag},
		Parameters: []Parameter{pathParam("uuid")}, Responses: jsonOK("SwitchCorrelated with enrichment"),
	})
	b.AddOperation("/api/v1/correlated/logical-routers", "get", &Operation{
		OperationID: "listCorrelatedRouters", Summary: "List correlated logical routers", Tags: []string{tag},
		Responses: jsonOK("Array of RouterCorrelated"),
	})
	b.AddOperation("/api/v1/correlated/logical-routers/{uuid}", "get", &Operation{
		OperationID: "getCorrelatedRouter", Summary: "Get correlated logical router by UUID", Tags: []string{tag},
		Parameters: []Parameter{pathParam("uuid")}, Responses: jsonOK("RouterCorrelated with enrichment"),
	})
	b.AddOperation("/api/v1/correlated/logical-switch-ports/{uuid}", "get", &Operation{
		OperationID: "getCorrelatedLSP", Summary: "Get correlated logical switch port", Tags: []string{tag},
		Parameters: []Parameter{pathParam("uuid")}, Responses: jsonOK("PortBindingChain"),
	})
	b.AddOperation("/api/v1/correlated/logical-router-ports/{uuid}", "get", &Operation{
		OperationID: "getCorrelatedLRP", Summary: "Get correlated logical router port", Tags: []string{tag},
		Parameters: []Parameter{pathParam("uuid")}, Responses: jsonOK("PortBindingChain"),
	})
	b.AddOperation("/api/v1/correlated/chassis", "get", &Operation{
		OperationID: "listCorrelatedChassis", Summary: "List correlated chassis", Tags: []string{tag},
		Responses: jsonOK("Array of ChassisCorrelated"),
	})
	b.AddOperation("/api/v1/correlated/chassis/{uuid}", "get", &Operation{
		OperationID: "getCorrelatedChassis", Summary: "Get correlated chassis by UUID", Tags: []string{tag},
		Parameters: []Parameter{pathParam("uuid")}, Responses: jsonOK("ChassisCorrelated"),
	})
	b.AddOperation("/api/v1/correlated/port-bindings/{uuid}", "get", &Operation{
		OperationID: "getCorrelatedPortBinding", Summary: "Get correlated port binding", Tags: []string{tag},
		Parameters: []Parameter{pathParam("uuid")}, Responses: jsonOK("PortBindingChain"),
	})
}

func registerSearch(b *Builder) {
	b.AddOperation("/api/v1/search", "get", &Operation{
		OperationID: "search",
		Summary:     "Search across NB and SB databases",
		Tags:        []string{"Search"},
		Parameters:  []Parameter{requiredQueryParam("q", "Search query (UUID, IP, MAC, or text)")},
		Responses:   jsonOK("Search results with query classification"),
	})
}

func registerTopology(b *Builder) {
	b.AddOperation("/api/v1/topology", "get", &Operation{
		OperationID: "getTopology",
		Summary:     "Get network topology graph",
		Tags:        []string{"Topology"},
		Parameters: []Parameter{
			queryParam("vms", "Include VM port nodes (true/false)"),
			queryParam("format", "Set to 'download' for Content-Disposition attachment"),
		},
		Responses: jsonOK("Topology graph with nodes and edges"),
	})
}

func registerFlows(b *Builder) {
	tag := "Flows"
	b.AddOperation("/api/v1/flows", "get", &Operation{
		OperationID: "getFlowPipeline",
		Summary:     "Get flow pipeline for a datapath",
		Tags:        []string{tag},
		Parameters:  []Parameter{requiredQueryParam("datapath", "Datapath binding UUID")},
		Responses:   jsonOK("Flow pipeline grouped by ingress/egress tables"),
	})
}

func registerDebug(b *Builder) {
	tag := "Debug"
	b.AddOperation("/api/v1/debug/port-diagnostics", "get", &Operation{
		OperationID: "listPortDiagnostics",
		Summary:     "Run diagnostics on all ports",
		Tags:        []string{tag},
		Responses:   jsonOK("Port diagnostics summary"),
	})
	b.AddOperation("/api/v1/debug/port-diagnostics/{uuid}", "get", &Operation{
		OperationID: "getPortDiagnostic",
		Summary:     "Run diagnostics on a single port",
		Tags:        []string{tag},
		Parameters:  []Parameter{pathParam("uuid")},
		Responses:   jsonOK("Single port diagnostic"),
	})
	b.AddOperation("/api/v1/debug/connectivity", "get", &Operation{
		OperationID: "checkConnectivity",
		Summary:     "Check connectivity between two ports",
		Tags:        []string{tag},
		Parameters: []Parameter{
			requiredQueryParam("src", "Source port UUID"),
			requiredQueryParam("dst", "Destination port UUID"),
		},
		Responses: jsonOK("Connectivity check result"),
	})
	b.AddOperation("/api/v1/debug/trace", "get", &Operation{
		OperationID: "tracePacket",
		Summary:     "Trace packet through flow pipeline",
		Tags:        []string{tag},
		Parameters: []Parameter{
			requiredQueryParam("port", "Port binding UUID"),
			queryParam("dst_ip", "Destination IP for heuristic matching"),
			queryParam("protocol", "Protocol for heuristic matching"),
		},
		Responses: jsonOK("Traced flow stages with heuristic matching"),
	})
	b.AddOperation("/api/v1/debug/flow-diff", "get", &Operation{
		OperationID: "getFlowDiff",
		Summary:     "Get recent flow changes",
		Tags:        []string{tag},
		Parameters: []Parameter{
			queryParam("datapath", "Filter by datapath UUID"),
			queryParam("since", "Unix milliseconds timestamp"),
		},
		Responses: jsonOK("Flow changes with count"),
	})
}

func registerHistory(b *Builder) {
	tag := "History"
	b.AddOperation("/api/v1/snapshots", "get", &Operation{
		OperationID: "listSnapshots", Summary: "List all snapshots", Tags: []string{tag},
		Responses: jsonOK("Array of snapshot metadata"),
	})
	b.AddOperation("/api/v1/snapshots", "post", &Operation{
		OperationID: "createSnapshot", Summary: "Create a manual snapshot", Tags: []string{tag},
		RequestBody: jsonBody(), Responses: jsonCreated("Snapshot metadata"),
	})
	b.AddOperation("/api/v1/snapshots/diff", "get", &Operation{
		OperationID: "diffSnapshots", Summary: "Diff two snapshots", Tags: []string{tag},
		Parameters: []Parameter{
			requiredQueryParam("from", "Source snapshot ID"),
			requiredQueryParam("to", "Target snapshot ID"),
			queryParam("table", "Filter by database.table"),
		},
		Responses: jsonOK("Diff result with field-level changes"),
	})
	b.AddOperation("/api/v1/snapshots/{id}", "get", &Operation{
		OperationID: "getSnapshot", Summary: "Get snapshot metadata", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, Responses: jsonOK("Snapshot metadata"),
	})
	b.AddOperation("/api/v1/snapshots/{id}/rows", "get", &Operation{
		OperationID: "getSnapshotRows", Summary: "Get snapshot rows", Tags: []string{tag},
		Parameters: []Parameter{
			pathParam("id"),
			queryParam("database", "Filter by database (nb/sb)"),
			queryParam("table", "Filter by table name"),
		},
		Responses: jsonOK("Array of snapshot rows"),
	})
	b.AddOperation("/api/v1/snapshots/{id}/export", "get", &Operation{
		OperationID: "exportSnapshot", Summary: "Export snapshot as JSON download", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, Responses: jsonOK("Snapshot export with Content-Disposition"),
	})
	b.AddOperation("/api/v1/snapshots/{id}", "delete", &Operation{
		OperationID: "deleteSnapshot", Summary: "Delete a snapshot", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, Responses: noContent(),
	})
	b.AddOperation("/api/v1/snapshots/import", "post", &Operation{
		OperationID: "importSnapshot", Summary: "Import a previously exported snapshot", Tags: []string{tag},
		RequestBody: jsonBody(), Responses: jsonCreated("Imported snapshot metadata"),
	})
	b.AddOperation("/api/v1/events", "get", &Operation{
		OperationID: "queryEvents", Summary: "Query event log", Tags: []string{tag},
		Parameters: []Parameter{
			queryParam("database", "Filter by database"),
			queryParam("table", "Filter by table name"),
			queryParam("type", "Filter by event type (insert/update/delete)"),
			queryParam("since", "Start time (RFC3339 or Unix millis)"),
			queryParam("until", "End time (RFC3339 or Unix millis)"),
			queryParam("limit", "Max events to return"),
		},
		Responses: jsonOK("Array of events"),
	})
}

func registerAlerts(b *Builder) {
	tag := "Alerts"
	b.AddOperation("/api/v1/alerts", "get", &Operation{
		OperationID: "listActiveAlerts", Summary: "List active alerts", Tags: []string{tag},
		Responses: jsonOK("Array of active alerts"),
	})
	b.AddOperation("/api/v1/alerts/rules", "get", &Operation{
		OperationID: "listAlertRules", Summary: "List alert rules", Tags: []string{tag},
		Responses: jsonOK("Array of alert rules with enabled status"),
	})
	b.AddOperation("/api/v1/alerts/rules/{name}", "put", &Operation{
		OperationID: "setRuleEnabled", Summary: "Enable or disable an alert rule", Tags: []string{tag},
		Parameters: []Parameter{pathParam("name")}, RequestBody: jsonBody(), Responses: jsonOK("Updated rule status"),
	})
	b.AddOperation("/api/v1/alerts/silences", "get", &Operation{
		OperationID: "listSilences", Summary: "List active silences", Tags: []string{tag},
		Responses: jsonOK("Array of silences"),
	})
	b.AddOperation("/api/v1/alerts/silences", "post", &Operation{
		OperationID: "createSilence", Summary: "Create an alert silence", Tags: []string{tag},
		RequestBody: jsonBody(), Responses: jsonCreated("Created silence"),
	})
	b.AddOperation("/api/v1/alerts/silences/{id}", "delete", &Operation{
		OperationID: "deleteSilence", Summary: "Delete an alert silence", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, Responses: noContent(),
	})
}

func registerTelemetry(b *Builder) {
	tag := "Telemetry"
	b.AddOperation("/api/v1/telemetry/summary", "get", &Operation{
		OperationID: "getTelemetrySummary", Summary: "Get telemetry health summary", Tags: []string{tag},
		Responses: jsonOK("Health summary with connection status and entity counts"),
	})
	b.AddOperation("/api/v1/telemetry/flows", "get", &Operation{
		OperationID: "getFlowMetrics", Summary: "Get flow count metrics", Tags: []string{tag},
		Responses: jsonOK("Flow metrics by pipeline and table"),
	})
	b.AddOperation("/api/v1/telemetry/propagation", "get", &Operation{
		OperationID: "getPropagation", Summary: "Get NbCfg propagation status", Tags: []string{tag},
		Responses: jsonOK("Configuration propagation chain"),
	})
	b.AddOperation("/api/v1/telemetry/cluster", "get", &Operation{
		OperationID: "getClusterStatus", Summary: "Get OVSDB cluster status", Tags: []string{tag},
		Responses: jsonOK("Cluster connection status"),
	})
	b.AddOperation("/metrics", "get", &Operation{
		OperationID: "getPrometheusMetrics",
		Summary:     "Prometheus metrics scrape endpoint",
		Tags:        []string{tag},
		Responses: map[string]Response{
			"200": {Description: "Prometheus metrics in text exposition format"},
		},
	})
}

func registerWrite(b *Builder) {
	tag := "Write"
	b.AddOperation("/api/v1/write/preview", "post", &Operation{
		OperationID: "previewWrite", Summary: "Preview write operations with diffs", Tags: []string{tag},
		RequestBody: jsonBody(), Responses: jsonOK("Plan with diffs and apply token"),
	})
	b.AddOperation("/api/v1/write/dry-run", "post", &Operation{
		OperationID: "dryRunWrite", Summary: "Dry-run write operations (no snapshot)", Tags: []string{tag},
		RequestBody: jsonBody(), Responses: jsonOK("Plan with diffs"),
	})
	b.AddOperation("/api/v1/write/plans/{id}", "get", &Operation{
		OperationID: "getWritePlan", Summary: "Get a pending write plan", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, Responses: jsonOK("Plan details"),
	})
	b.AddOperation("/api/v1/write/plans/{id}/apply", "post", &Operation{
		OperationID: "applyWritePlan", Summary: "Apply a previewed write plan", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, RequestBody: jsonBody(), Responses: jsonOK("Audit entry"),
	})
	b.AddOperation("/api/v1/write/plans/{id}", "delete", &Operation{
		OperationID: "cancelWritePlan", Summary: "Cancel a pending write plan", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, Responses: noContent(),
	})
	b.AddOperation("/api/v1/write/rollback", "post", &Operation{
		OperationID: "rollbackWrite", Summary: "Rollback to a snapshot (not yet implemented)", Tags: []string{tag},
		RequestBody: jsonBody(), Responses: map[string]Response{
			"501": {Description: "Not implemented"},
		},
	})
	b.AddOperation("/api/v1/write/audit", "get", &Operation{
		OperationID: "listWriteAudit", Summary: "List write audit entries", Tags: []string{tag},
		Parameters: []Parameter{queryParam("limit", "Max entries to return (default 100)")},
		Responses:  jsonOK("Array of audit entries"),
	})
	b.AddOperation("/api/v1/write/audit/{id}", "get", &Operation{
		OperationID: "getWriteAuditEntry", Summary: "Get a write audit entry by ID", Tags: []string{tag},
		Parameters: []Parameter{pathParam("id")}, Responses: jsonOK("Audit entry"),
	})
}

func registerWebSocket(b *Builder) {
	b.AddOperation("/api/v1/ws", "get", &Operation{
		OperationID: "websocket",
		Summary:     "WebSocket endpoint for real-time events",
		Description: "Upgrades to WebSocket. Send subscribe/unsubscribe messages to filter events by database and table.",
		Tags:        []string{"Events"},
		Responses: map[string]Response{
			"101": {Description: "Switching Protocols (WebSocket upgrade)"},
		},
	})
}
