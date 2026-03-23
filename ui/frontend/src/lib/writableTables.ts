export interface WritableTable {
  ovsdbName: string;
  slug: string;
  label: string;
  deleteOnly?: boolean;
}

export const writableTables: WritableTable[] = [
  {
    ovsdbName: 'Logical_Switch',
    slug: 'logical-switches',
    label: 'Logical Switches',
  },
  {
    ovsdbName: 'Logical_Switch_Port',
    slug: 'logical-switch-ports',
    label: 'Logical Switch Ports',
  },
  {
    ovsdbName: 'Logical_Router',
    slug: 'logical-routers',
    label: 'Logical Routers',
  },
  {
    ovsdbName: 'Logical_Router_Port',
    slug: 'logical-router-ports',
    label: 'Logical Router Ports',
  },
  { ovsdbName: 'ACL', slug: 'acls', label: 'ACLs' },
  { ovsdbName: 'NAT', slug: 'nats', label: 'NAT' },
  {
    ovsdbName: 'Address_Set',
    slug: 'address-sets',
    label: 'Address Sets',
  },
  { ovsdbName: 'Port_Group', slug: 'port-groups', label: 'Port Groups' },
  {
    ovsdbName: 'Load_Balancer',
    slug: 'load-balancers',
    label: 'Load Balancers',
  },
  {
    ovsdbName: 'Logical_Router_Static_Route',
    slug: 'logical-router-static-routes',
    label: 'Static Routes',
  },
  {
    ovsdbName: 'Logical_Router_Policy',
    slug: 'logical-router-policies',
    label: 'Router Policies',
  },
  { ovsdbName: 'DHCP_Options', slug: 'dhcp-options', label: 'DHCP Options' },
  { ovsdbName: 'DNS', slug: 'dns', label: 'DNS' },
  {
    ovsdbName: 'Static_MAC_Binding',
    slug: 'static-mac-bindings',
    label: 'Static MAC Bindings',
  },
  { ovsdbName: 'HA_Chassis', slug: 'ha-chassis', label: 'HA Chassis' },
  {
    ovsdbName: 'Gateway_Chassis',
    slug: 'gateway-chassis',
    label: 'Gateway Chassis',
  },
  // SB tables — delete-only (stale entry cleanup)
  {
    ovsdbName: 'MAC_Binding',
    slug: 'mac-bindings',
    label: 'MAC Bindings',
    deleteOnly: true,
  },
  { ovsdbName: 'FDB', slug: 'fdb', label: 'FDB', deleteOnly: true },
  {
    ovsdbName: 'Port_Binding',
    slug: 'port-bindings',
    label: 'Port Bindings',
    deleteOnly: true,
  },
];

export function isWritableTable(ovsdbName: string): boolean {
  return writableTables.some((t) => t.ovsdbName === ovsdbName);
}

export function findWritableTable(
  ovsdbName: string,
): WritableTable | undefined {
  return writableTables.find((t) => t.ovsdbName === ovsdbName);
}
