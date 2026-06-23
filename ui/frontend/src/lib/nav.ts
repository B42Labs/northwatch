// Single source of truth for the curated navigation. Drives BOTH the sidebar
// and the Home launcher, which previously hand-listed the same links twice.

export interface NavLink {
  label: string;
  href: string;
  /** Match only on exact path (for parents whose children share the prefix). */
  exact?: boolean;
}

export interface NavSection {
  key: string;
  label: string;
  /** Short blurb shown on the Home launcher cards. */
  description: string;
  links: NavLink[];
  /** Only shown when the backend has write operations enabled. */
  requiresWrite?: boolean;
}

export const navSections: NavSection[] = [
  {
    key: 'correlated',
    label: 'Correlated',
    description: 'Enriched NB/SB views with cross-database correlation.',
    links: [
      { label: 'Logical Switches', href: '/correlated/logical-switches' },
      { label: 'Logical Routers', href: '/correlated/logical-routers' },
      { label: 'Chassis', href: '/correlated/chassis' },
    ],
  },
  {
    key: 'visualization',
    label: 'Visualize',
    description: 'Topology, flows, and network-policy views.',
    links: [
      { label: 'Topology', href: '/topology' },
      { label: 'Flow Pipeline', href: '/flows' },
      { label: 'Security Policy', href: '/security' },
      { label: 'NAT Overview', href: '/nat' },
      { label: 'HA Failover', href: '/ha' },
      { label: 'MAC Table', href: '/mac-table' },
      { label: 'Load Balancers', href: '/load-balancers' },
    ],
  },
  {
    key: 'debug',
    label: 'Debug',
    description: 'Packet tracing, diagnostics, and audit tools.',
    links: [
      { label: 'Packet Trace', href: '/debug/trace' },
      { label: 'Flow Diff', href: '/debug/flow-diff' },
      { label: 'Connectivity', href: '/debug/connectivity' },
      { label: 'Port Diagnostics', href: '/debug/port-diagnostics' },
      { label: 'ACL Audit', href: '/debug/acl-audit' },
      { label: 'Stale Entries', href: '/debug/stale-entries' },
    ],
  },
  {
    key: 'history',
    label: 'History & Events',
    description: 'Database change events and state snapshots.',
    links: [
      { label: 'Events', href: '/events' },
      { label: 'Snapshots', href: '/history' },
    ],
  },
  {
    key: 'monitoring',
    label: 'Monitoring',
    description: 'Cluster health and RAFT consensus status.',
    links: [
      { label: 'Raft Health', href: '/raft-health' },
      { label: 'Propagation Timeline', href: '/propagation-timeline' },
    ],
  },
  {
    key: 'write',
    label: 'Write',
    description: 'Create, update, or delete OVN entities.',
    requiresWrite: true,
    links: [
      { label: 'Operation Builder', href: '/write', exact: true },
      { label: 'Audit Log', href: '/write/audit' },
    ],
  },
];

/** Whether `href` is the active route for the current `path`. */
export function isActiveLink(path: string, link: NavLink): boolean {
  if (link.exact) return path === link.href;
  return path === link.href || path.startsWith(link.href + '/');
}
