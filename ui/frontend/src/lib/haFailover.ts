// Pure resolution logic for the HA-failover view: it matches Northbound and
// Southbound HA groups by chassis-name membership, elects the NB leader, detects
// drained chassis, finds the SB-active chassis from chassisredirect port
// bindings, and folds all of that into the per-group failover chains the route
// renders. Kept free of Svelte (no runes, no fetch) so the logic is
// unit-testable in isolation, mirroring lib/ovsHealth.ts and lib/ovsCorrelate.ts.

import type { Variant } from './status';

// --- NB types — primary data source for group structure and names. ---
// The backend write operations work with NB data, so API calls must use NB
// group names and NB chassis names.

export interface NbHaChassisEntry {
  _uuid: string;
  chassis_name: string;
  priority: number;
  external_ids: Record<string, string>;
}

export interface NbHaChassisGroup {
  _uuid: string;
  name: string;
  ha_chassis: string[];
  external_ids: Record<string, string>;
}

export interface NbGatewayChassisEntry {
  _uuid: string;
  chassis_name: string;
  name: string;
  priority: number;
  external_ids: Record<string, string>;
}

export interface NbLogicalRouterPort {
  _uuid: string;
  name: string;
  gateway_chassis: string[] | null;
  ha_chassis_group: string | null;
}

// --- SB types — used only for determining the actual active chassis (from port
// bindings) and chassis hostnames. ---

export interface SbHaChassisEntry {
  _uuid: string;
  chassis: string | null;
  priority: number;
  external_ids: Record<string, string>;
}

export interface SbHaChassisGroup {
  _uuid: string;
  name: string;
  ha_chassis: string[];
  external_ids: Record<string, string>;
}

export interface ChassisRecord {
  _uuid: string;
  name: string;
  hostname: string;
}

export interface PortBindingRecord {
  _uuid: string;
  type: string;
  logical_port: string;
  chassis: string | null;
  ha_chassis_group: string | null;
}

// --- Resolved (display) types ---

export interface ResolvedChassisEntry {
  uuid: string;
  chassisUuid: string | null;
  chassisName: string;
  hostname: string;
  priority: number;
  isActive: boolean;
  isDrained: boolean;
}

export interface ResolvedGroup {
  uuid: string;
  name: string;
  nbGroupName: string | null; // NB group name for write API calls (null if NB has no matching group)
  nbLeaderName: string | null; // chassis with highest NB priority (what the backend considers "active")
  chassisChain: ResolvedChassisEntry[];
  crPortName: string | null;
  activeChassis: string | null; // SB-active chassis UUID (actual runtime)
}

export interface ActiveChassisInfo {
  name: string;
  hostname: string;
  activeInGroups: string[];
}

export interface DrainedChassisInfo {
  name: string;
  hostname: string;
  drainedInGroups: string[];
}

/** NbGroupInfo maps a chassis-name-key back to the NB group name (for write API
 * calls) and its elected leader (what the backend treats as "active"). */
export interface NbGroupInfo {
  groupName: string;
  leaderName: string;
}

/** HaFailoverData bundles the eight table fetches the route performs. */
export interface HaFailoverData {
  sbHaGroups: SbHaChassisGroup[];
  sbHaChassis: SbHaChassisEntry[];
  sbChassisList: ChassisRecord[];
  sbPortBindings: PortBindingRecord[];
  nbHaGroups: NbHaChassisGroup[];
  nbHaChassis: NbHaChassisEntry[];
  nbGwChassis: NbGatewayChassisEntry[];
  nbLrps: NbLogicalRouterPort[];
}

/** ResolvedHaState is the fully-derived view state the route assigns to its
 * runes after a load. */
export interface ResolvedHaState {
  groups: ResolvedGroup[];
  totalChassisInvolved: number;
  drainedChassisInfo: DrainedChassisInfo[];
  hasNbGroups: boolean;
}

// --- Pure derivation functions ---

/** chassisNameKey builds a stable key from sorted chassis names for matching NB
 * and SB groups by membership. */
export function chassisNameKey(names: string[]): string {
  return [...names].sort().join('\0');
}

/** findLeader returns the chassis_name with the highest priority (the NB
 * leader). Empty input yields an empty string. */
export function findLeader(
  entries: { chassis_name: string; priority: number }[],
): string {
  if (entries.length === 0) return '';
  let best = entries[0];
  for (const e of entries) {
    if (e.priority > best.priority) best = e;
  }
  return best.chassis_name;
}

/** buildNbGroupInfo maps chassis-name-key -> { groupName, leaderName } from both
 * NB HA_Chassis_Groups and Gateway_Chassis (via LRPs). The backend resolves
 * groups from both, so either source lets the UI map an SB group to the NB name
 * the write API expects. */
export function buildNbGroupInfo(
  nbHaGroups: NbHaChassisGroup[],
  nbHaChassis: NbHaChassisEntry[],
  nbGwChassis: NbGatewayChassisEntry[],
  nbLrps: NbLogicalRouterPort[],
): Map<string, NbGroupInfo> {
  const nbKeyToGroupInfo = new Map<string, NbGroupInfo>();

  // From NB HA_Chassis_Groups.
  const nbChassisMap = new Map<string, NbHaChassisEntry>();
  for (const hc of nbHaChassis) {
    nbChassisMap.set(hc._uuid, hc);
  }
  for (const nbGroup of nbHaGroups) {
    const entries: NbHaChassisEntry[] = [];
    for (const hcUuid of nbGroup.ha_chassis) {
      const hc = nbChassisMap.get(hcUuid);
      if (hc?.chassis_name) entries.push(hc);
    }
    if (entries.length > 0) {
      const key = chassisNameKey(entries.map((e) => e.chassis_name));
      nbKeyToGroupInfo.set(key, {
        groupName: nbGroup.name,
        leaderName: findLeader(entries),
      });
    }
  }

  // From NB Gateway_Chassis (via LRPs).
  const nbGwChassisMap = new Map<string, NbGatewayChassisEntry>();
  for (const gw of nbGwChassis) {
    nbGwChassisMap.set(gw._uuid, gw);
  }
  for (const lrp of nbLrps) {
    if (!lrp.gateway_chassis || lrp.gateway_chassis.length === 0) continue;
    const entries: NbGatewayChassisEntry[] = [];
    for (const gwUuid of lrp.gateway_chassis) {
      const gw = nbGwChassisMap.get(gwUuid);
      if (gw?.chassis_name) entries.push(gw);
    }
    if (entries.length > 0) {
      const key = chassisNameKey(entries.map((e) => e.chassis_name));
      nbKeyToGroupInfo.set(key, {
        groupName: lrp.name,
        leaderName: findLeader(entries),
      });
    }
  }

  return nbKeyToGroupInfo;
}

/** detectDrainedChassis finds chassis parked at priority 0 by an evacuation:
 * they carry the `northwatch:pre-drain-priority` external-id recording their
 * original priority. A priority-0 chassis without that marker is not treated as
 * drained. Returns one entry per drained chassis, sorted by how many groups it
 * is drained in (most first). */
export function detectDrainedChassis(
  nbHaGroups: NbHaChassisGroup[],
  nbHaChassis: NbHaChassisEntry[],
  nbGwChassis: NbGatewayChassisEntry[],
  nbLrps: NbLogicalRouterPort[],
  hostnameByName: Map<string, string>,
): DrainedChassisInfo[] {
  const nbChassisMap = new Map<string, NbHaChassisEntry>();
  for (const hc of nbHaChassis) {
    nbChassisMap.set(hc._uuid, hc);
  }
  const nbGwChassisMap = new Map<string, NbGatewayChassisEntry>();
  for (const gw of nbGwChassis) {
    nbGwChassisMap.set(gw._uuid, gw);
  }

  const drainedMap = new Map<string, DrainedChassisInfo>();

  function recordDrained(
    chassisName: string,
    groupName: string,
    externalIds: Record<string, string> | undefined,
  ) {
    if (!externalIds?.['northwatch:pre-drain-priority']) return;
    const existing = drainedMap.get(chassisName);
    if (existing) {
      existing.drainedInGroups.push(groupName);
    } else {
      drainedMap.set(chassisName, {
        name: chassisName,
        hostname: hostnameByName.get(chassisName) || '',
        drainedInGroups: [groupName],
      });
    }
  }

  for (const nbGroup of nbHaGroups) {
    for (const hcUuid of nbGroup.ha_chassis) {
      const hc = nbChassisMap.get(hcUuid);
      if (hc && hc.priority === 0) {
        recordDrained(hc.chassis_name, nbGroup.name, hc.external_ids);
      }
    }
  }

  for (const lrp of nbLrps) {
    if (!lrp.gateway_chassis || lrp.gateway_chassis.length === 0) continue;
    for (const gwUuid of lrp.gateway_chassis) {
      const gw = nbGwChassisMap.get(gwUuid);
      if (gw && gw.priority === 0) {
        recordDrained(gw.chassis_name, lrp.name, gw.external_ids);
      }
    }
  }

  return [...drainedMap.values()].sort(
    (a, b) => b.drainedInGroups.length - a.drainedInGroups.length,
  );
}

/** buildActiveChassisMap reduces the chassisredirect Port_Bindings to two maps
 * keyed by SB HA-group UUID: the active chassis UUID (Port_Binding.chassis) and
 * the CR port name. Only bound chassisredirect ports contribute. */
export function buildActiveChassisMap(sbPortBindings: PortBindingRecord[]): {
  active: Map<string, string>;
  crPort: Map<string, string>;
} {
  const active = new Map<string, string>();
  const crPort = new Map<string, string>();
  for (const pb of sbPortBindings) {
    if (pb.type === 'chassisredirect' && pb.ha_chassis_group && pb.chassis) {
      active.set(pb.ha_chassis_group, pb.chassis);
      crPort.set(pb.ha_chassis_group, pb.logical_port);
    }
  }
  return { active, crPort };
}

/** resolveHaGroups folds all eight tables into the display state: SB drives the
 * groups and chains, NB supplies the write-API names/leader, and the
 * chassisredirect bindings mark the actual active chassis. */
export function resolveHaGroups(data: HaFailoverData): ResolvedHaState {
  const {
    sbHaGroups,
    sbHaChassis,
    sbChassisList,
    sbPortBindings,
    nbHaGroups,
    nbHaChassis,
    nbGwChassis,
    nbLrps,
  } = data;

  // --- SB lookups (for display) ---
  const sbHaChassisMap = new Map<string, SbHaChassisEntry>();
  for (const hc of sbHaChassis) {
    sbHaChassisMap.set(hc._uuid, hc);
  }

  const sbChassisMap = new Map<string, ChassisRecord>();
  for (const ch of sbChassisList) {
    sbChassisMap.set(ch._uuid, ch);
  }

  // Chassisredirect port bindings: SB HA group UUID -> active chassis + CR port.
  const { active: haGroupToActiveChassis, crPort: haGroupToCrPort } =
    buildActiveChassisMap(sbPortBindings);

  // --- NB lookups (for write API name mapping) ---
  const nbKeyToGroupInfo = buildNbGroupInfo(
    nbHaGroups,
    nbHaChassis,
    nbGwChassis,
    nbLrps,
  );

  // --- Detect drained chassis from NB data ---
  const chassisHostnameByName = new Map<string, string>();
  for (const ch of sbChassisList) {
    chassisHostnameByName.set(ch.name, ch.hostname);
  }

  const drainedChassisInfo = detectDrainedChassis(
    nbHaGroups,
    nbHaChassis,
    nbGwChassis,
    nbLrps,
    chassisHostnameByName,
  );
  const drainedNames = new Set(drainedChassisInfo.map((d) => d.name));

  const hasNbGroups = nbKeyToGroupInfo.size > 0;

  // --- Resolve SB groups for display, with NB name mapping ---
  const chassisUuidsInvolved = new Set<string>();

  const resolved: ResolvedGroup[] = sbHaGroups.map((group) => {
    const activeChassis = haGroupToActiveChassis.get(group._uuid) ?? null;
    const crPortName = haGroupToCrPort.get(group._uuid) ?? null;

    // Resolve SB HA chassis entries for this group.
    const memberChassisNames: string[] = [];
    const chainEntries: ResolvedChassisEntry[] = group.ha_chassis
      .map((hcUuid) => {
        const hc = sbHaChassisMap.get(hcUuid);
        if (!hc) return null;

        const chassisRecord = hc.chassis ? sbChassisMap.get(hc.chassis) : null;

        if (hc.chassis) {
          chassisUuidsInvolved.add(hc.chassis);
        }
        if (chassisRecord?.name) {
          memberChassisNames.push(chassisRecord.name);
        }

        const chassisName = chassisRecord?.name ?? hc.chassis ?? 'unknown';
        return {
          uuid: hc._uuid,
          chassisUuid: hc.chassis,
          chassisName,
          hostname: chassisRecord?.hostname ?? '',
          priority: hc.priority,
          isActive: hc.chassis === activeChassis && activeChassis !== null,
          isDrained: drainedNames.has(chassisName),
        } satisfies ResolvedChassisEntry;
      })
      .filter((e): e is ResolvedChassisEntry => e !== null)
      .sort((a, b) => b.priority - a.priority);

    // Look up matching NB group info by chassis name membership.
    const key =
      memberChassisNames.length > 0 ? chassisNameKey(memberChassisNames) : '';
    const nbInfo = key ? (nbKeyToGroupInfo.get(key) ?? null) : null;

    return {
      uuid: group._uuid,
      name: group.name,
      nbGroupName: nbInfo?.groupName ?? null,
      nbLeaderName: nbInfo?.leaderName ?? null,
      chassisChain: chainEntries,
      crPortName,
      activeChassis,
    };
  });

  // Sort groups by name.
  resolved.sort((a, b) => a.name.localeCompare(b.name));

  return {
    groups: resolved,
    totalChassisInvolved: chassisUuidsInvolved.size,
    drainedChassisInfo,
    hasNbGroups,
  };
}

/** computeActiveChassisInfo folds the resolved groups into the per-chassis
 * evacuate picker: one row per chassis that is active somewhere, with the groups
 * it leads, sorted by how many it is active in (most first). */
export function computeActiveChassisInfo(
  groups: ResolvedGroup[],
): ActiveChassisInfo[] {
  const map = new Map<string, ActiveChassisInfo>();
  for (const g of groups) {
    const active = g.chassisChain.find((c) => c.isActive);
    if (active) {
      const existing = map.get(active.chassisName);
      if (existing) {
        existing.activeInGroups.push(g.name);
      } else {
        map.set(active.chassisName, {
          name: active.chassisName,
          hostname: active.hostname,
          activeInGroups: [g.name],
        });
      }
    }
  }
  return [...map.values()].sort(
    (a, b) => b.activeInGroups.length - a.activeInGroups.length,
  );
}

/** filterGroups keeps groups whose name, CR port, or any member chassis
 * name/hostname contains the (case-insensitive) query. A blank query returns all
 * groups unchanged. */
export function filterGroups(
  groups: ResolvedGroup[],
  query: string,
): ResolvedGroup[] {
  if (!query.trim()) return groups;
  const q = query.toLowerCase();
  return groups.filter((g) => {
    if (g.name.toLowerCase().includes(q)) return true;
    if (g.crPortName?.toLowerCase().includes(q)) return true;
    return g.chassisChain.some(
      (c) =>
        c.chassisName.toLowerCase().includes(q) ||
        c.hostname.toLowerCase().includes(q),
    );
  });
}

/** entryStatus classifies a chassis-chain entry into its status badge: ACTIVE
 * (the bound gateway), DRAINED (parked at priority 0 by an evacuation), or
 * STANDBY. The highest-priority standby of a group with no active chassis is a
 * warning; every other standby is muted (ghost). */
export function entryStatus(
  entry: ResolvedChassisEntry,
  idx: number,
  hasActiveChassis: boolean,
): { label: 'ACTIVE' | 'DRAINED' | 'STANDBY'; variant: Variant } {
  if (entry.isActive) return { label: 'ACTIVE', variant: 'success' };
  if (entry.isDrained) return { label: 'DRAINED', variant: 'error' };
  if (idx === 0 && !hasActiveChassis)
    return { label: 'STANDBY', variant: 'warning' };
  return { label: 'STANDBY', variant: 'ghost' };
}

/** shortName truncates long chassis/group names for the chain boxes. */
export function shortName(name: string): string {
  if (name.length <= 32) return name;
  return name.slice(0, 29) + '...';
}
