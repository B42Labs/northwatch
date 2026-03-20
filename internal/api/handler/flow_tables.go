package handler

// ovnTableNames maps well-known OVN pipeline table IDs to human-readable stage names.
// These are fallback values used only when the flow's ExternalIDs["stage-name"] is absent.
// Table IDs may vary across OVN versions — prefer stage-name from ExternalIDs when available.
var ovnTableNames = map[int]string{
	0:  "Admission Control",
	1:  "Ingress Port Security - L2",
	2:  "Ingress Port Security - IP",
	3:  "Ingress Port Security - ND",
	4:  "Pre-ACL",
	5:  "Pre-LB",
	6:  "Pre-Stateful",
	7:  "ACL Hints",
	8:  "ACL",
	9:  "ACL Action",
	10: "QoS",
	11: "Stateful",
	12: "DHCP Options",
	13: "DHCP Response",
	14: "DNS Lookup",
	15: "DNS Response",
	16: "External Port Mapping",
	17: "L2 Lookup",
	18: "L2 Unknown",
	19: "L3 Lookup",
	20: "MAC Learning",
}

// OVNTableName returns the human-readable name for a well-known OVN table ID.
// Returns empty string if the table ID is not in the static map.
func OVNTableName(tableID int) string {
	return ovnTableNames[tableID]
}
