import { describe, it, expect } from 'vitest';
import {
  chassisNameKey,
  findLeader,
  buildNbGroupInfo,
  detectDrainedChassis,
  buildActiveChassisMap,
  resolveHaGroups,
  computeActiveChassisInfo,
  filterGroups,
  entryStatus,
  shortName,
  type NbHaChassisEntry,
  type NbHaChassisGroup,
  type NbGatewayChassisEntry,
  type NbLogicalRouterPort,
  type SbHaChassisEntry,
  type SbHaChassisGroup,
  type ChassisRecord,
  type PortBindingRecord,
  type ResolvedChassisEntry,
  type ResolvedGroup,
  type HaFailoverData,
} from './haFailover';

// --- Factories -----------------------------------------------------------

function chassisEntry(
  o: Partial<ResolvedChassisEntry> = {},
): ResolvedChassisEntry {
  return {
    uuid: 'sh-x',
    chassisUuid: 'c-x',
    chassisName: 'node-x',
    hostname: 'host-x',
    priority: 10,
    isActive: false,
    isDrained: false,
    ...o,
  };
}

function group(o: Partial<ResolvedGroup> = {}): ResolvedGroup {
  return {
    uuid: 'g-x',
    name: 'group-x',
    nbGroupName: null,
    nbLeaderName: null,
    chassisChain: [],
    crPortName: null,
    activeChassis: null,
    ...o,
  };
}

// A two-chassis (node-a > node-b) deployment with matching NB and SB groups and
// an active chassisredirect binding on node-a. Individual tests override slices.
function fixture(): HaFailoverData {
  const chassisList: ChassisRecord[] = [
    { _uuid: 'ca', name: 'node-a', hostname: 'host-a' },
    { _uuid: 'cb', name: 'node-b', hostname: 'host-b' },
  ];
  const sbHaChassis: SbHaChassisEntry[] = [
    { _uuid: 'sh1', chassis: 'ca', priority: 20, external_ids: {} },
    { _uuid: 'sh2', chassis: 'cb', priority: 10, external_ids: {} },
  ];
  const sbHaGroups: SbHaChassisGroup[] = [
    {
      _uuid: 'sg1',
      name: 'sb-group-1',
      ha_chassis: ['sh1', 'sh2'],
      external_ids: {},
    },
  ];
  const sbPortBindings: PortBindingRecord[] = [
    {
      _uuid: 'pb1',
      type: 'chassisredirect',
      logical_port: 'cr-lrp1',
      chassis: 'ca',
      ha_chassis_group: 'sg1',
    },
  ];
  const nbHaChassis: NbHaChassisEntry[] = [
    { _uuid: 'nh1', chassis_name: 'node-a', priority: 20, external_ids: {} },
    { _uuid: 'nh2', chassis_name: 'node-b', priority: 10, external_ids: {} },
  ];
  const nbHaGroups: NbHaChassisGroup[] = [
    {
      _uuid: 'ng1',
      name: 'nb-group-1',
      ha_chassis: ['nh1', 'nh2'],
      external_ids: {},
    },
  ];
  return {
    sbHaGroups,
    sbHaChassis,
    sbChassisList: chassisList,
    sbPortBindings,
    nbHaGroups,
    nbHaChassis,
    nbGwChassis: [],
    nbLrps: [],
  };
}

// --- chassisNameKey ------------------------------------------------------

describe('chassisNameKey', () => {
  it('is order-independent (sorts before joining)', () => {
    expect(chassisNameKey(['b', 'a', 'c'])).toBe(
      chassisNameKey(['c', 'a', 'b']),
    );
    expect(chassisNameKey(['a', 'b'])).toBe('a\0b');
  });

  it('returns an empty string for empty input', () => {
    expect(chassisNameKey([])).toBe('');
  });
});

// --- findLeader ----------------------------------------------------------

describe('findLeader', () => {
  it('returns the chassis with the highest priority', () => {
    expect(
      findLeader([
        { chassis_name: 'a', priority: 5 },
        { chassis_name: 'b', priority: 30 },
        { chassis_name: 'c', priority: 10 },
      ]),
    ).toBe('b');
  });

  it('keeps the first winner on a priority tie', () => {
    expect(
      findLeader([
        { chassis_name: 'a', priority: 10 },
        { chassis_name: 'b', priority: 10 },
      ]),
    ).toBe('a');
  });

  it('returns an empty string for empty input', () => {
    expect(findLeader([])).toBe('');
  });
});

// --- buildNbGroupInfo ----------------------------------------------------

describe('buildNbGroupInfo', () => {
  it('maps the sorted-membership key to the NB group name and leader', () => {
    const f = fixture();
    const map = buildNbGroupInfo(
      f.nbHaGroups,
      f.nbHaChassis,
      f.nbGwChassis,
      f.nbLrps,
    );
    const info = map.get(chassisNameKey(['node-a', 'node-b']));
    expect(info).toEqual({ groupName: 'nb-group-1', leaderName: 'node-a' });
  });

  it('sources groups from Gateway_Chassis via LRPs too', () => {
    const nbGwChassis: NbGatewayChassisEntry[] = [
      {
        _uuid: 'gw1',
        name: 'gw1',
        chassis_name: 'node-a',
        priority: 5,
        external_ids: {},
      },
      {
        _uuid: 'gw2',
        name: 'gw2',
        chassis_name: 'node-b',
        priority: 8,
        external_ids: {},
      },
    ];
    const nbLrps: NbLogicalRouterPort[] = [
      {
        _uuid: 'lrp1',
        name: 'lrp-gw',
        gateway_chassis: ['gw1', 'gw2'],
        ha_chassis_group: null,
      },
    ];
    const map = buildNbGroupInfo([], [], nbGwChassis, nbLrps);
    const info = map.get(chassisNameKey(['node-a', 'node-b']));
    // node-b has the higher gateway_chassis priority, so it leads.
    expect(info).toEqual({ groupName: 'lrp-gw', leaderName: 'node-b' });
  });

  it('skips LRPs with no gateway_chassis and empty groups', () => {
    const nbLrps: NbLogicalRouterPort[] = [
      {
        _uuid: 'lrp0',
        name: 'lrp-empty',
        gateway_chassis: null,
        ha_chassis_group: null,
      },
      {
        _uuid: 'lrp1',
        name: 'lrp-none',
        gateway_chassis: [],
        ha_chassis_group: null,
      },
    ];
    expect(buildNbGroupInfo([], [], [], nbLrps).size).toBe(0);
  });

  it('returns an empty map for empty input', () => {
    expect(buildNbGroupInfo([], [], [], []).size).toBe(0);
  });
});

// --- detectDrainedChassis ------------------------------------------------

describe('detectDrainedChassis', () => {
  const hostnames = new Map([
    ['node-a', 'host-a'],
    ['node-c', 'host-c'],
  ]);

  it('flags a priority-0 chassis carrying the pre-drain-priority marker', () => {
    const nbHaChassis: NbHaChassisEntry[] = [
      {
        _uuid: 'nh3',
        chassis_name: 'node-c',
        priority: 0,
        external_ids: { 'northwatch:pre-drain-priority': '20' },
      },
    ];
    const nbHaGroups: NbHaChassisGroup[] = [
      {
        _uuid: 'ng1',
        name: 'nb-group-1',
        ha_chassis: ['nh3'],
        external_ids: {},
      },
    ];
    const drained = detectDrainedChassis(
      nbHaGroups,
      nbHaChassis,
      [],
      [],
      hostnames,
    );
    expect(drained).toEqual([
      { name: 'node-c', hostname: 'host-c', drainedInGroups: ['nb-group-1'] },
    ]);
  });

  it('does not flag a priority-0 chassis without the marker', () => {
    const nbHaChassis: NbHaChassisEntry[] = [
      { _uuid: 'nh3', chassis_name: 'node-c', priority: 0, external_ids: {} },
    ];
    const nbHaGroups: NbHaChassisGroup[] = [
      {
        _uuid: 'ng1',
        name: 'nb-group-1',
        ha_chassis: ['nh3'],
        external_ids: {},
      },
    ];
    expect(
      detectDrainedChassis(nbHaGroups, nbHaChassis, [], [], hostnames),
    ).toEqual([]);
  });

  it('does not flag a marked chassis whose priority is not 0', () => {
    const nbHaChassis: NbHaChassisEntry[] = [
      {
        _uuid: 'nh3',
        chassis_name: 'node-c',
        priority: 5,
        external_ids: { 'northwatch:pre-drain-priority': '20' },
      },
    ];
    const nbHaGroups: NbHaChassisGroup[] = [
      {
        _uuid: 'ng1',
        name: 'nb-group-1',
        ha_chassis: ['nh3'],
        external_ids: {},
      },
    ];
    expect(
      detectDrainedChassis(nbHaGroups, nbHaChassis, [], [], hostnames),
    ).toEqual([]);
  });

  it('detects drained gateway_chassis via LRPs and sorts by group count', () => {
    const nbGwChassis: NbGatewayChassisEntry[] = [
      {
        _uuid: 'gw-a',
        name: 'gw-a',
        chassis_name: 'node-a',
        priority: 0,
        external_ids: { 'northwatch:pre-drain-priority': '10' },
      },
      {
        _uuid: 'gw-c',
        name: 'gw-c',
        chassis_name: 'node-c',
        priority: 0,
        external_ids: { 'northwatch:pre-drain-priority': '5' },
      },
    ];
    const nbLrps: NbLogicalRouterPort[] = [
      {
        _uuid: 'lrp1',
        name: 'lrp-1',
        gateway_chassis: ['gw-a', 'gw-c'],
        ha_chassis_group: null,
      },
      {
        _uuid: 'lrp2',
        name: 'lrp-2',
        gateway_chassis: ['gw-a'],
        ha_chassis_group: null,
      },
    ];
    const drained = detectDrainedChassis(
      [],
      [],
      nbGwChassis,
      nbLrps,
      hostnames,
    );
    // node-a is drained in two LRPs, node-c in one -> node-a first.
    expect(drained.map((d) => d.name)).toEqual(['node-a', 'node-c']);
    expect(drained[0].drainedInGroups).toEqual(['lrp-1', 'lrp-2']);
    expect(drained[1]).toEqual({
      name: 'node-c',
      hostname: 'host-c',
      drainedInGroups: ['lrp-1'],
    });
  });

  it('falls back to an empty hostname when unknown', () => {
    const nbHaChassis: NbHaChassisEntry[] = [
      {
        _uuid: 'nh',
        chassis_name: 'ghost-node',
        priority: 0,
        external_ids: { 'northwatch:pre-drain-priority': '1' },
      },
    ];
    const nbHaGroups: NbHaChassisGroup[] = [
      { _uuid: 'ng', name: 'g', ha_chassis: ['nh'], external_ids: {} },
    ];
    const drained = detectDrainedChassis(
      nbHaGroups,
      nbHaChassis,
      [],
      [],
      new Map(),
    );
    expect(drained[0].hostname).toBe('');
  });

  it('returns an empty list for empty input', () => {
    expect(detectDrainedChassis([], [], [], [], new Map())).toEqual([]);
  });
});

// --- buildActiveChassisMap -----------------------------------------------

describe('buildActiveChassisMap', () => {
  it('maps a bound chassisredirect binding to its active chassis and CR port', () => {
    const { active, crPort } = buildActiveChassisMap([
      {
        _uuid: 'pb1',
        type: 'chassisredirect',
        logical_port: 'cr-lrp1',
        chassis: 'ca',
        ha_chassis_group: 'sg1',
      },
    ]);
    expect(active.get('sg1')).toBe('ca');
    expect(crPort.get('sg1')).toBe('cr-lrp1');
  });

  it('ignores non-chassisredirect ports and unbound / group-less bindings', () => {
    const { active } = buildActiveChassisMap([
      {
        _uuid: 'p1',
        type: 'patch',
        logical_port: 'p',
        chassis: 'ca',
        ha_chassis_group: 'sg1',
      },
      {
        _uuid: 'p2',
        type: 'chassisredirect',
        logical_port: 'cr',
        chassis: null,
        ha_chassis_group: 'sg1',
      },
      {
        _uuid: 'p3',
        type: 'chassisredirect',
        logical_port: 'cr',
        chassis: 'ca',
        ha_chassis_group: null,
      },
    ]);
    expect(active.size).toBe(0);
  });

  it('returns empty maps for empty input', () => {
    const { active, crPort } = buildActiveChassisMap([]);
    expect(active.size).toBe(0);
    expect(crPort.size).toBe(0);
  });
});

// --- resolveHaGroups -----------------------------------------------------

describe('resolveHaGroups', () => {
  it('resolves an SB group, maps it to the NB name/leader and marks the active chassis', () => {
    const state = resolveHaGroups(fixture());
    expect(state.groups).toHaveLength(1);
    const g = state.groups[0];
    expect(g.name).toBe('sb-group-1');
    expect(g.nbGroupName).toBe('nb-group-1');
    expect(g.nbLeaderName).toBe('node-a');
    expect(g.activeChassis).toBe('ca');
    expect(g.crPortName).toBe('cr-lrp1');
    // Chain is priority-sorted (highest first) with node-a active.
    expect(g.chassisChain.map((c) => c.chassisName)).toEqual([
      'node-a',
      'node-b',
    ]);
    expect(g.chassisChain[0].isActive).toBe(true);
    expect(g.chassisChain[1].isActive).toBe(false);
    expect(state.totalChassisInvolved).toBe(2);
    expect(state.hasNbGroups).toBe(true);
  });

  it('sorts the chain by priority even when SB lists entries out of order', () => {
    const f = fixture();
    // Swap the SB member order; resolveHaGroups must still put P20 first.
    f.sbHaGroups[0].ha_chassis = ['sh2', 'sh1'];
    const g = resolveHaGroups(f).groups[0];
    expect(g.chassisChain.map((c) => c.priority)).toEqual([20, 10]);
  });

  it('marks a group with no matching port binding as having no active chassis', () => {
    const f = fixture();
    f.sbPortBindings = []; // no chassisredirect binding -> nothing active
    const g = resolveHaGroups(f).groups[0];
    expect(g.activeChassis).toBeNull();
    expect(g.crPortName).toBeNull();
    expect(g.chassisChain.every((c) => !c.isActive)).toBe(true);
  });

  it('leaves nbGroupName null when no NB group matches the membership', () => {
    const f = fixture();
    f.nbHaGroups = [];
    f.nbHaChassis = [];
    const state = resolveHaGroups(f);
    expect(state.groups[0].nbGroupName).toBeNull();
    expect(state.groups[0].nbLeaderName).toBeNull();
    expect(state.hasNbGroups).toBe(false);
  });

  it('marks a drained chassis in the chain via NB pre-drain markers', () => {
    const f = fixture();
    // Drain node-b in NB: priority 0 + marker.
    f.nbHaChassis[1] = {
      _uuid: 'nh2',
      chassis_name: 'node-b',
      priority: 0,
      external_ids: { 'northwatch:pre-drain-priority': '10' },
    };
    const state = resolveHaGroups(f);
    const nodeB = state.groups[0].chassisChain.find(
      (c) => c.chassisName === 'node-b',
    );
    expect(nodeB?.isDrained).toBe(true);
    expect(state.drainedChassisInfo.map((d) => d.name)).toContain('node-b');
  });

  it('sorts multiple groups by name and handles unknown chassis records', () => {
    const f = fixture();
    f.sbHaGroups.push({
      _uuid: 'sg0',
      name: 'aaa-first',
      ha_chassis: ['sh-orphan'],
      external_ids: {},
    });
    // sh-orphan references a chassis with no ChassisRecord.
    f.sbHaChassis.push({
      _uuid: 'sh-orphan',
      chassis: 'c-missing',
      priority: 1,
      external_ids: {},
    });
    const state = resolveHaGroups(f);
    expect(state.groups.map((g) => g.name)).toEqual([
      'aaa-first',
      'sb-group-1',
    ]);
    // A missing ChassisRecord falls back to the raw chassis UUID for the name.
    expect(state.groups[0].chassisChain[0].chassisName).toBe('c-missing');
  });

  it('returns an empty state for empty input', () => {
    const state = resolveHaGroups({
      sbHaGroups: [],
      sbHaChassis: [],
      sbChassisList: [],
      sbPortBindings: [],
      nbHaGroups: [],
      nbHaChassis: [],
      nbGwChassis: [],
      nbLrps: [],
    });
    expect(state).toEqual({
      groups: [],
      totalChassisInvolved: 0,
      drainedChassisInfo: [],
      hasNbGroups: false,
    });
  });
});

// --- computeActiveChassisInfo --------------------------------------------

describe('computeActiveChassisInfo', () => {
  it('aggregates each active chassis across the groups it leads, most first', () => {
    const groups: ResolvedGroup[] = [
      group({
        name: 'g1',
        chassisChain: [chassisEntry({ chassisName: 'node-a', isActive: true })],
      }),
      group({
        name: 'g2',
        chassisChain: [chassisEntry({ chassisName: 'node-a', isActive: true })],
      }),
      group({
        name: 'g3',
        chassisChain: [chassisEntry({ chassisName: 'node-b', isActive: true })],
      }),
    ];
    const info = computeActiveChassisInfo(groups);
    expect(info.map((i) => i.name)).toEqual(['node-a', 'node-b']);
    expect(info[0].activeInGroups).toEqual(['g1', 'g2']);
    expect(info[1].activeInGroups).toEqual(['g3']);
  });

  it('ignores groups with no active member', () => {
    const groups: ResolvedGroup[] = [
      group({ chassisChain: [chassisEntry({ isActive: false })] }),
    ];
    expect(computeActiveChassisInfo(groups)).toEqual([]);
  });

  it('returns an empty list for empty input', () => {
    expect(computeActiveChassisInfo([])).toEqual([]);
  });
});

// --- filterGroups --------------------------------------------------------

describe('filterGroups', () => {
  const groups: ResolvedGroup[] = [
    group({
      uuid: 'g1',
      name: 'edge-router-gw',
      crPortName: 'cr-lrp-edge',
      chassisChain: [
        chassisEntry({ chassisName: 'node-a', hostname: 'compute-1' }),
      ],
    }),
    group({
      uuid: 'g2',
      name: 'internal',
      crPortName: null,
      chassisChain: [
        chassisEntry({ chassisName: 'node-z', hostname: 'compute-9' }),
      ],
    }),
  ];

  it('returns all groups for a blank query', () => {
    expect(filterGroups(groups, '   ')).toBe(groups);
  });

  it('matches on group name, CR port, chassis name and hostname (case-insensitive)', () => {
    expect(filterGroups(groups, 'EDGE').map((g) => g.uuid)).toEqual(['g1']);
    expect(filterGroups(groups, 'cr-lrp-edge').map((g) => g.uuid)).toEqual([
      'g1',
    ]);
    expect(filterGroups(groups, 'node-z').map((g) => g.uuid)).toEqual(['g2']);
    expect(filterGroups(groups, 'compute-9').map((g) => g.uuid)).toEqual([
      'g2',
    ]);
  });

  it('returns an empty list when nothing matches', () => {
    expect(filterGroups(groups, 'no-such-thing')).toEqual([]);
  });

  it('returns an empty list for empty input', () => {
    expect(filterGroups([], 'anything')).toEqual([]);
  });
});

// --- entryStatus ---------------------------------------------------------

describe('entryStatus', () => {
  it('classifies an active entry as ACTIVE/success', () => {
    expect(entryStatus(chassisEntry({ isActive: true }), 0, true)).toEqual({
      label: 'ACTIVE',
      variant: 'success',
    });
  });

  it('classifies a drained (non-active) entry as DRAINED/error', () => {
    expect(
      entryStatus(chassisEntry({ isActive: false, isDrained: true }), 1, true),
    ).toEqual({ label: 'DRAINED', variant: 'error' });
  });

  it('flags the top standby of a group with no active chassis as STANDBY/warning', () => {
    expect(entryStatus(chassisEntry(), 0, false)).toEqual({
      label: 'STANDBY',
      variant: 'warning',
    });
  });

  it('mutes every other standby as STANDBY/ghost', () => {
    // Not the first entry.
    expect(entryStatus(chassisEntry(), 1, false)).toEqual({
      label: 'STANDBY',
      variant: 'ghost',
    });
    // First entry but the group already has an active chassis.
    expect(entryStatus(chassisEntry(), 0, true)).toEqual({
      label: 'STANDBY',
      variant: 'ghost',
    });
  });
});

// --- shortName -----------------------------------------------------------

describe('shortName', () => {
  it('leaves short names untouched (<= 32 chars)', () => {
    expect(shortName('node-a')).toBe('node-a');
    expect(shortName('x'.repeat(32))).toBe('x'.repeat(32));
  });

  it('truncates long names to 29 chars plus an ellipsis', () => {
    const long = 'a'.repeat(40);
    expect(shortName(long)).toBe('a'.repeat(29) + '...');
    expect(shortName(long)).toHaveLength(32);
  });

  it('handles an empty string', () => {
    expect(shortName('')).toBe('');
  });
});
