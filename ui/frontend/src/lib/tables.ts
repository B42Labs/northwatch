export interface TableDef {
  slug: string;
  label: string;
  primaryColumns: string[];
  references?: Record<string, { db: string; table: string }>;
}

export interface DbDef {
  key: string;
  label: string;
  tables: TableDef[];
}

export const databases: DbDef[] = [
  {
    key: 'nb',
    label: 'Northbound',
    tables: [
      {
        slug: 'logical-switches',
        label: 'Logical Switches',
        primaryColumns: ['_uuid', 'name', 'ports', 'external_ids'],
        references: { ports: { db: 'nb', table: 'logical-switch-ports' } },
      },
      {
        slug: 'logical-switch-ports',
        label: 'Logical Switch Ports',
        primaryColumns: [
          '_uuid',
          'name',
          'type',
          'addresses',
          'up',
          'external_ids',
        ],
      },
      {
        slug: 'logical-routers',
        label: 'Logical Routers',
        primaryColumns: ['_uuid', 'name', 'ports', 'nat', 'external_ids'],
        references: {
          ports: { db: 'nb', table: 'logical-router-ports' },
          nat: { db: 'nb', table: 'nats' },
        },
      },
      {
        slug: 'logical-router-ports',
        label: 'Logical Router Ports',
        primaryColumns: ['_uuid', 'name', 'mac', 'networks', 'external_ids'],
      },
      {
        slug: 'acls',
        label: 'ACLs',
        primaryColumns: ['_uuid', 'priority', 'direction', 'match', 'action'],
      },
      {
        slug: 'nats',
        label: 'NAT',
        primaryColumns: [
          '_uuid',
          'type',
          'external_ip',
          'logical_ip',
          'external_ids',
        ],
      },
      {
        slug: 'address-sets',
        label: 'Address Sets',
        primaryColumns: ['_uuid', 'name', 'addresses'],
      },
      {
        slug: 'port-groups',
        label: 'Port Groups',
        primaryColumns: ['_uuid', 'name', 'ports', 'acls'],
        references: {
          ports: { db: 'nb', table: 'logical-switch-ports' },
          acls: { db: 'nb', table: 'acls' },
        },
      },
      {
        slug: 'load-balancers',
        label: 'Load Balancers',
        primaryColumns: ['_uuid', 'name', 'vips', 'protocol'],
      },
      {
        slug: 'load-balancer-groups',
        label: 'Load Balancer Groups',
        primaryColumns: ['_uuid', 'name', 'load_balancer'],
      },
      {
        slug: 'logical-router-policies',
        label: 'Router Policies',
        primaryColumns: ['_uuid', 'priority', 'match', 'action'],
      },
      {
        slug: 'logical-router-static-routes',
        label: 'Static Routes',
        primaryColumns: ['_uuid', 'ip_prefix', 'nexthop', 'output_port'],
      },
      {
        slug: 'dhcp-options',
        label: 'DHCP Options',
        primaryColumns: ['_uuid', 'cidr', 'options', 'external_ids'],
      },
      {
        slug: 'nb-global',
        label: 'NB Global',
        primaryColumns: ['_uuid', 'nb_cfg', 'sb_cfg', 'hv_cfg', 'external_ids'],
      },
      {
        slug: 'connections',
        label: 'Connections',
        primaryColumns: ['_uuid', 'target', 'is_connected', 'status'],
      },
      {
        slug: 'dns',
        label: 'DNS',
        primaryColumns: ['_uuid', 'records', 'external_ids'],
      },
      {
        slug: 'gateway-chassis',
        label: 'Gateway Chassis',
        primaryColumns: ['_uuid', 'name', 'chassis_name', 'priority'],
      },
      {
        slug: 'ha-chassis-groups',
        label: 'HA Chassis Groups',
        primaryColumns: ['_uuid', 'name', 'ha_chassis'],
      },
      {
        slug: 'meters',
        label: 'Meters',
        primaryColumns: ['_uuid', 'name', 'unit', 'bands'],
      },
      {
        slug: 'qos',
        label: 'QoS',
        primaryColumns: ['_uuid', 'priority', 'direction', 'match'],
      },
      {
        slug: 'bfd',
        label: 'BFD',
        primaryColumns: ['_uuid', 'dst_ip', 'logical_port', 'status'],
      },
      {
        slug: 'copp',
        label: 'CoPP',
        primaryColumns: ['_uuid', 'name', 'meters'],
      },
      {
        slug: 'mirrors',
        label: 'Mirrors',
        primaryColumns: ['_uuid', 'name', 'type', 'sink'],
      },
      {
        slug: 'forwarding-groups',
        label: 'Forwarding Groups',
        primaryColumns: ['_uuid', 'name', 'vip', 'vmac'],
      },
      {
        slug: 'static-mac-bindings',
        label: 'Static MAC Bindings',
        primaryColumns: ['_uuid', 'logical_port', 'ip', 'mac'],
      },
      {
        slug: 'load-balancer-health-checks',
        label: 'LB Health Checks',
        primaryColumns: ['_uuid', 'vip', 'options'],
      },
    ],
  },
  {
    key: 'sb',
    label: 'Southbound',
    tables: [
      {
        slug: 'chassis',
        label: 'Chassis',
        primaryColumns: ['_uuid', 'name', 'hostname', 'encaps', 'external_ids'],
        references: { encaps: { db: 'sb', table: 'encaps' } },
      },
      {
        slug: 'port-bindings',
        label: 'Port Bindings',
        primaryColumns: [
          '_uuid',
          'logical_port',
          'type',
          'datapath',
          'chassis',
          'tunnel_key',
          'mac',
        ],
      },
      {
        slug: 'datapath-bindings',
        label: 'Datapath Bindings',
        primaryColumns: ['_uuid', 'tunnel_key', 'external_ids'],
      },
      {
        slug: 'logical-flows',
        label: 'Logical Flows',
        primaryColumns: [
          '_uuid',
          'logical_datapath',
          'pipeline',
          'table_id',
          'priority',
          'match',
          'actions',
        ],
      },
      {
        slug: 'encaps',
        label: 'Encaps',
        primaryColumns: ['_uuid', 'type', 'ip', 'chassis_name', 'options'],
      },
      {
        slug: 'mac-bindings',
        label: 'MAC Bindings',
        primaryColumns: ['_uuid', 'logical_port', 'ip', 'mac', 'datapath'],
      },
      {
        slug: 'fdb',
        label: 'FDB',
        primaryColumns: ['_uuid', 'mac', 'dp_key', 'port_key'],
      },
      {
        slug: 'multicast-groups',
        label: 'Multicast Groups',
        primaryColumns: ['_uuid', 'name', 'datapath', 'tunnel_key', 'ports'],
      },
      {
        slug: 'address-sets',
        label: 'Address Sets',
        primaryColumns: ['_uuid', 'name', 'addresses'],
      },
      {
        slug: 'port-groups',
        label: 'Port Groups',
        primaryColumns: ['_uuid', 'name', 'ports'],
      },
      {
        slug: 'load-balancers',
        label: 'Load Balancers',
        primaryColumns: ['_uuid', 'name', 'vips', 'protocol', 'datapaths'],
      },
      {
        slug: 'dns',
        label: 'DNS',
        primaryColumns: ['_uuid', 'records', 'datapaths', 'external_ids'],
      },
      {
        slug: 'sb-global',
        label: 'SB Global',
        primaryColumns: ['_uuid', 'nb_cfg', 'external_ids', 'options'],
      },
      {
        slug: 'connections',
        label: 'Connections',
        primaryColumns: ['_uuid', 'target', 'is_connected', 'status'],
      },
      {
        slug: 'gateway-chassis',
        label: 'Gateway Chassis',
        primaryColumns: ['_uuid', 'name', 'chassis', 'priority'],
      },
      {
        slug: 'ha-chassis-groups',
        label: 'HA Chassis Groups',
        primaryColumns: ['_uuid', 'name', 'ha_chassis'],
      },
      {
        slug: 'ip-multicast',
        label: 'IP Multicast',
        primaryColumns: ['_uuid', 'datapath', 'enabled', 'querier'],
      },
      {
        slug: 'igmp-groups',
        label: 'IGMP Groups',
        primaryColumns: ['_uuid', 'address', 'datapath', 'chassis', 'ports'],
      },
      {
        slug: 'service-monitors',
        label: 'Service Monitors',
        primaryColumns: [
          '_uuid',
          'ip',
          'port',
          'protocol',
          'status',
          'logical_port',
        ],
      },
      {
        slug: 'bfd',
        label: 'BFD',
        primaryColumns: [
          '_uuid',
          'src_port',
          'disc',
          'dst_ip',
          'status',
          'logical_port',
        ],
      },
      {
        slug: 'meters',
        label: 'Meters',
        primaryColumns: ['_uuid', 'name', 'unit', 'bands'],
      },
      {
        slug: 'mirrors',
        label: 'Mirrors',
        primaryColumns: ['_uuid', 'name', 'type', 'sink'],
      },
      {
        slug: 'chassis-private',
        label: 'Chassis Private',
        primaryColumns: ['_uuid', 'name', 'nb_cfg', 'nb_cfg_timestamp'],
      },
      {
        slug: 'controller-events',
        label: 'Controller Events',
        primaryColumns: ['_uuid', 'event_type', 'event_info'],
      },
      {
        slug: 'static-mac-bindings',
        label: 'Static MAC Bindings',
        primaryColumns: ['_uuid', 'logical_port', 'ip', 'mac'],
      },
      {
        slug: 'logical-dp-groups',
        label: 'Logical DP Groups',
        primaryColumns: ['_uuid', 'datapaths'],
      },
      {
        slug: 'rbac-roles',
        label: 'RBAC Roles',
        primaryColumns: ['_uuid', 'name', 'permissions'],
      },
      {
        slug: 'rbac-permissions',
        label: 'RBAC Permissions',
        primaryColumns: [
          '_uuid',
          'table',
          'authorization',
          'insert_delete',
          'update',
        ],
      },
    ],
  },
];

// Correlated views for the sidebar
export const correlatedViews = [
  { slug: 'logical-switches', label: 'Logical Switches' },
  { slug: 'logical-routers', label: 'Logical Routers' },
  { slug: 'chassis', label: 'Chassis' },
];

// Find a table definition by db key and slug
export function findTable(dbKey: string, slug: string): TableDef | undefined {
  const db = databases.find((d) => d.key === dbKey);
  return db?.tables.find((t) => t.slug === slug);
}

// Convert an OVSDB table name (e.g. "Logical_Switch") to a URL slug (e.g. "logical-switches").
// Uses the known table definitions for an exact match; falls back to a regex conversion.
// WHEN ADDING A NEW TABLE: add it to the `databases` array above, then add its
// OVSDB name -> slug mapping below. The auto-generated loop handles simple cases
// (e.g. "Logical_Switches" -> "logical-switches"), but manual overrides are needed
// for: pluralization mismatches (Encap -> encaps), acronyms (ACL, NAT, DNS),
// and irregular singular/plural forms (Logical_Switch -> logical-switches).
const ovsdbNameToSlug: Map<string, string> = new Map();
for (const db of databases) {
  for (const table of db.tables) {
    // OVSDB names: convert slug back to the OVSDB convention
    // e.g. "logical-switches" -> "Logical_Switch" (singular in OVSDB)
    // We build the reverse map from all known slugs so lookups are O(1).
    const ovsdbName = table.slug
      .split('-')
      .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
      .join('_');
    ovsdbNameToSlug.set(ovsdbName, table.slug);
  }
}
// Manual overrides for irregular OVSDB names that don't match the slug pattern
ovsdbNameToSlug.set('Logical_Switch', 'logical-switches');
ovsdbNameToSlug.set('Logical_Switch_Port', 'logical-switch-ports');
ovsdbNameToSlug.set('Logical_Router', 'logical-routers');
ovsdbNameToSlug.set('Logical_Router_Port', 'logical-router-ports');
ovsdbNameToSlug.set('Logical_Router_Policy', 'logical-router-policies');
ovsdbNameToSlug.set(
  'Logical_Router_Static_Route',
  'logical-router-static-routes',
);
ovsdbNameToSlug.set('Port_Binding', 'port-bindings');
ovsdbNameToSlug.set('Datapath_Binding', 'datapath-bindings');
ovsdbNameToSlug.set('Logical_Flow', 'logical-flows');
ovsdbNameToSlug.set('MAC_Binding', 'mac-bindings');
ovsdbNameToSlug.set('Multicast_Group', 'multicast-groups');
ovsdbNameToSlug.set('Address_Set', 'address-sets');
ovsdbNameToSlug.set('Port_Group', 'port-groups');
ovsdbNameToSlug.set('Load_Balancer', 'load-balancers');
ovsdbNameToSlug.set('Load_Balancer_Group', 'load-balancer-groups');
ovsdbNameToSlug.set(
  'Load_Balancer_Health_Check',
  'load-balancer-health-checks',
);
ovsdbNameToSlug.set('DHCP_Options', 'dhcp-options');
ovsdbNameToSlug.set('NB_Global', 'nb-global');
ovsdbNameToSlug.set('SB_Global', 'sb-global');
ovsdbNameToSlug.set('Gateway_Chassis', 'gateway-chassis');
ovsdbNameToSlug.set('HA_Chassis_Group', 'ha-chassis-groups');
ovsdbNameToSlug.set('Chassis_Private', 'chassis-private');
ovsdbNameToSlug.set('Controller_Event', 'controller-events');
ovsdbNameToSlug.set('Static_MAC_Binding', 'static-mac-bindings');
ovsdbNameToSlug.set('IP_Multicast', 'ip-multicast');
ovsdbNameToSlug.set('IGMP_Group', 'igmp-groups');
ovsdbNameToSlug.set('Service_Monitor', 'service-monitors');
ovsdbNameToSlug.set('Logical_DP_Group', 'logical-dp-groups');
ovsdbNameToSlug.set('RBAC_Role', 'rbac-roles');
ovsdbNameToSlug.set('RBAC_Permission', 'rbac-permissions');
ovsdbNameToSlug.set('Forwarding_Group', 'forwarding-groups');
// Short/all-caps OVSDB names where fallback produces wrong slug
ovsdbNameToSlug.set('ACL', 'acls');
ovsdbNameToSlug.set('NAT', 'nats');
ovsdbNameToSlug.set('DNS', 'dns');
ovsdbNameToSlug.set('FDB', 'fdb');
ovsdbNameToSlug.set('BFD', 'bfd');
ovsdbNameToSlug.set('QoS', 'qos');
// Singular OVSDB names that map to plural slugs
ovsdbNameToSlug.set('Encap', 'encaps');
ovsdbNameToSlug.set('Meter', 'meters');
ovsdbNameToSlug.set('Mirror', 'mirrors');
ovsdbNameToSlug.set('Copp', 'copp');

export function tableSlugFromOvsdbName(name: string): string {
  const slug = ovsdbNameToSlug.get(name);
  if (slug) return slug;
  // Fallback for unknown tables
  return name
    .replace(/_/g, '-')
    .replace(/([a-z])([A-Z])/g, '$1-$2')
    .toLowerCase();
}

// Get the correlated route for a raw table entity, if available
export function getCorrelatedRoute(db: string, table: string): string | null {
  const map: Record<string, string> = {
    'nb:logical-switches': '/correlated/logical-switches',
    'nb:logical-switch-ports': '/correlated/logical-switch-ports',
    'nb:logical-routers': '/correlated/logical-routers',
    'nb:logical-router-ports': '/correlated/logical-router-ports',
    'sb:chassis': '/correlated/chassis',
    'sb:port-bindings': '/correlated/port-bindings',
  };
  return map[`${db}:${table}`] || null;
}
