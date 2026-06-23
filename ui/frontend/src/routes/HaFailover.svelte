<script lang="ts">
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { writeEnabled } from '../lib/capabilitiesStore';
  import {
    requestFailover,
    requestEvacuate,
    requestRestore,
    applyPlan,
    cancelPlan,
    type Plan,
  } from '../lib/writeApi';

  // --- Types ---

  // NB types — primary data source for group structure and names.
  // The backend write operations work with NB data, so API calls must
  // use NB group names and NB chassis names.
  interface NbHaChassisEntry {
    _uuid: string;
    chassis_name: string;
    priority: number;
    external_ids: Record<string, string>;
  }

  interface NbHaChassisGroup {
    _uuid: string;
    name: string;
    ha_chassis: string[];
    external_ids: Record<string, string>;
  }

  interface NbGatewayChassisEntry {
    _uuid: string;
    chassis_name: string;
    name: string;
    priority: number;
    external_ids: Record<string, string>;
  }

  interface NbLogicalRouterPort {
    _uuid: string;
    name: string;
    gateway_chassis: string[] | null;
    ha_chassis_group: string | null;
  }

  // SB types — used only for determining the actual active chassis
  // (from port bindings) and chassis hostnames.
  interface SbHaChassisEntry {
    _uuid: string;
    chassis: string | null;
    priority: number;
    external_ids: Record<string, string>;
  }

  interface SbHaChassisGroup {
    _uuid: string;
    name: string;
    ha_chassis: string[];
    external_ids: Record<string, string>;
  }

  interface ChassisRecord {
    _uuid: string;
    name: string;
    hostname: string;
  }

  interface PortBindingRecord {
    _uuid: string;
    type: string;
    logical_port: string;
    chassis: string | null;
    ha_chassis_group: string | null;
  }

  interface ResolvedChassisEntry {
    uuid: string;
    chassisUuid: string | null;
    chassisName: string;
    hostname: string;
    priority: number;
    isActive: boolean;
    isDrained: boolean;
  }

  interface ResolvedGroup {
    uuid: string;
    name: string;
    nbGroupName: string | null; // NB group name for write API calls (null if NB has no matching group)
    nbLeaderName: string | null; // chassis with highest NB priority (what the backend considers "active")
    chassisChain: ResolvedChassisEntry[];
    crPortName: string | null;
    activeChassis: string | null; // SB-active chassis UUID (actual runtime)
  }

  // --- Fetch helper ---

  async function fetchJson<T>(path: string): Promise<T> {
    const res = await fetch(path);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  // --- State ---

  interface SelectionPanelOpts {
    title: string;
    description: string;
    countHeader: string;
    rows: { name: string; hostname: string; count: number }[];
    countVariant: (count: number) => 'neutral' | 'warning' | 'error';
    actionLabel: string;
    actionClass: string;
    onaction: (name: string) => void;
    onclose: () => void;
  }

  let loading = $state(true);
  let error = $state('');
  let groups: ResolvedGroup[] = $state([]);
  let totalChassisInvolved = $state(0);
  let searchQuery = $state('');
  let hasNbGroups = $state(false);

  // --- Failover/Evacuate state ---

  let failoverTarget: {
    groupName: string;
    targetChassis: string;
    activeChassisName: string;
  } | null = $state(null);
  let evacuateTarget: string | null = $state(null);
  let restoreTarget: string | null = $state(null);
  let showEvacuateDropdown = $state(false);
  let showRestoreDropdown = $state(false);
  let pendingPlan: Plan | null = $state(null);
  let actionLoading = $state(false);
  let actionError = $state('');
  let actionSuccess = $state('');

  // --- Derived: active chassis with context for evacuate panel ---

  interface ActiveChassisInfo {
    name: string;
    hostname: string;
    activeInGroups: string[];
  }

  interface DrainedChassisInfo {
    name: string;
    hostname: string;
    drainedInGroups: string[];
  }

  let drainedChassisInfo: DrainedChassisInfo[] = $state([]);

  let activeChassisInfo = $derived.by(() => {
    const map = new SvelteMap<string, ActiveChassisInfo>();
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
  });

  // --- Filtered groups ---

  let filteredGroups = $derived.by(() => {
    if (!searchQuery.trim()) return groups;
    const q = searchQuery.toLowerCase();
    return groups.filter((g) => {
      if (g.name.toLowerCase().includes(q)) return true;
      if (g.crPortName?.toLowerCase().includes(q)) return true;
      return g.chassisChain.some(
        (c) =>
          c.chassisName.toLowerCase().includes(q) ||
          c.hostname.toLowerCase().includes(q),
      );
    });
  });

  // --- Load data ---

  // chassisNameKey builds a stable key from sorted chassis names for matching
  // NB and SB groups by membership.
  function chassisNameKey(names: string[]): string {
    return [...names].sort().join('\0');
  }

  async function load() {
    loading = true;
    error = '';
    try {
      // Display is always driven by SB data (guaranteed to exist).
      // NB data is fetched additionally to enable write operations —
      // the backend works on NB HA_Chassis_Groups or Gateway_Chassis,
      // so we map SB groups to NB groups by chassis name membership.
      const [
        sbHaGroups,
        sbHaChassis,
        sbChassisList,
        sbPortBindings,
        nbHaGroups,
        nbHaChassis,
        nbGwChassis,
        nbLrps,
      ] = await Promise.all([
        fetchJson<SbHaChassisGroup[]>('/api/v1/sb/ha-chassis-groups'),
        fetchJson<SbHaChassisEntry[]>('/api/v1/sb/ha-chassis'),
        fetchJson<ChassisRecord[]>('/api/v1/sb/chassis'),
        fetchJson<PortBindingRecord[]>('/api/v1/sb/port-bindings'),
        fetchJson<NbHaChassisGroup[]>('/api/v1/nb/ha-chassis-groups'),
        fetchJson<NbHaChassisEntry[]>('/api/v1/nb/ha-chassis'),
        fetchJson<NbGatewayChassisEntry[]>('/api/v1/nb/gateway-chassis'),
        fetchJson<NbLogicalRouterPort[]>('/api/v1/nb/logical-router-ports'),
      ]);

      // --- SB lookups (for display) ---

      const sbHaChassisMap = new SvelteMap<string, SbHaChassisEntry>();
      for (const hc of sbHaChassis) {
        sbHaChassisMap.set(hc._uuid, hc);
      }

      const sbChassisMap = new SvelteMap<string, ChassisRecord>();
      for (const ch of sbChassisList) {
        sbChassisMap.set(ch._uuid, ch);
      }

      // Chassisredirect port bindings: SB HA group UUID -> active chassis UUID + CR port
      const haGroupToActiveChassis = new SvelteMap<string, string>();
      const haGroupToCrPort = new SvelteMap<string, string>();
      for (const pb of sbPortBindings) {
        if (
          pb.type === 'chassisredirect' &&
          pb.ha_chassis_group &&
          pb.chassis
        ) {
          haGroupToActiveChassis.set(pb.ha_chassis_group, pb.chassis);
          haGroupToCrPort.set(pb.ha_chassis_group, pb.logical_port);
        }
      }

      // --- NB lookups (for write API name mapping) ---
      // Build chassis-name-key -> NB group name from both HA_Chassis_Groups
      // and Gateway_Chassis (via LRPs). The backend resolves groups from both.

      // chassis-name-key -> { groupName, leaderName }
      const nbKeyToGroupInfo = new SvelteMap<
        string,
        { groupName: string; leaderName: string }
      >();

      // Helper: find chassis with highest priority (NB leader)
      function findLeader(
        entries: { chassis_name: string; priority: number }[],
      ): string {
        if (entries.length === 0) return '';
        let best = entries[0];
        for (const e of entries) {
          if (e.priority > best.priority) best = e;
        }
        return best.chassis_name;
      }

      // From NB HA_Chassis_Groups
      const nbChassisMap = new SvelteMap<string, NbHaChassisEntry>();
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

      // From NB Gateway_Chassis (via LRPs)
      const nbGwChassisMap = new SvelteMap<string, NbGatewayChassisEntry>();
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

      // --- Detect drained chassis from NB data ---
      const chassisHostnameByName = new SvelteMap<string, string>();
      for (const ch of sbChassisList) {
        chassisHostnameByName.set(ch.name, ch.hostname);
      }

      const drainedMap = new SvelteMap<string, DrainedChassisInfo>();

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
            hostname: chassisHostnameByName.get(chassisName) || '',
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

      drainedChassisInfo = [...drainedMap.values()].sort(
        (a, b) => b.drainedInGroups.length - a.drainedInGroups.length,
      );

      hasNbGroups = nbKeyToGroupInfo.size > 0;

      // --- Resolve SB groups for display, with NB name mapping ---

      const chassisUuidsInvolved = new SvelteSet<string>();

      const resolved: ResolvedGroup[] = sbHaGroups.map((group) => {
        const activeChassis = haGroupToActiveChassis.get(group._uuid) ?? null;
        const crPortName = haGroupToCrPort.get(group._uuid) ?? null;

        // Resolve SB HA chassis entries for this group
        const memberChassisNames: string[] = [];
        const chainEntries: ResolvedChassisEntry[] = group.ha_chassis
          .map((hcUuid) => {
            const hc = sbHaChassisMap.get(hcUuid);
            if (!hc) return null;

            const chassisRecord = hc.chassis
              ? sbChassisMap.get(hc.chassis)
              : null;

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
              isDrained: drainedMap.has(chassisName),
            } satisfies ResolvedChassisEntry;
          })
          .filter((e): e is ResolvedChassisEntry => e !== null)
          .sort((a, b) => b.priority - a.priority);

        // Look up matching NB group info by chassis name membership
        const key =
          memberChassisNames.length > 0
            ? chassisNameKey(memberChassisNames)
            : '';
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

      // Sort groups by name
      resolved.sort((a, b) => a.name.localeCompare(b.name));

      groups = resolved;
      totalChassisInvolved = chassisUuidsInvolved.size;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load HA data';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load();
  });

  function shortName(name: string): string {
    if (name.length <= 32) return name;
    return name.slice(0, 29) + '...';
  }

  // --- Failover actions ---

  function clearAction() {
    failoverTarget = null;
    evacuateTarget = null;
    restoreTarget = null;
    showEvacuateDropdown = false;
    showRestoreDropdown = false;
    pendingPlan = null;
    actionLoading = false;
    actionError = '';
    actionSuccess = '';
  }

  async function startFailover(
    groupName: string,
    targetChassis: string,
    activeChassisName: string,
  ) {
    clearAction();
    failoverTarget = { groupName, targetChassis, activeChassisName };
    actionLoading = true;
    actionError = '';
    try {
      pendingPlan = await requestFailover({
        group_name: groupName,
        target_chassis: targetChassis,
      });
    } catch (e) {
      actionError =
        e instanceof Error ? e.message : 'Failed to preview failover';
    } finally {
      actionLoading = false;
    }
  }

  async function startEvacuate(chassisName: string) {
    clearAction();
    evacuateTarget = chassisName;
    actionLoading = true;
    actionError = '';
    try {
      pendingPlan = await requestEvacuate({ chassis_name: chassisName });
    } catch (e) {
      actionError =
        e instanceof Error ? e.message : 'Failed to preview evacuation';
    } finally {
      actionLoading = false;
    }
  }

  async function startRestore(chassisName: string) {
    clearAction();
    restoreTarget = chassisName;
    actionLoading = true;
    actionError = '';
    try {
      pendingPlan = await requestRestore({ chassis_name: chassisName });
    } catch (e) {
      actionError =
        e instanceof Error ? e.message : 'Failed to preview restore';
    } finally {
      actionLoading = false;
    }
  }

  async function confirmApply() {
    if (!pendingPlan) return;
    actionLoading = true;
    actionError = '';
    try {
      await applyPlan(pendingPlan.id, pendingPlan.apply_token, 'northwatch-ui');
      actionSuccess = failoverTarget
        ? `Failover completed: ${failoverTarget.activeChassisName} \u2192 ${failoverTarget.targetChassis}`
        : evacuateTarget
          ? `Evacuation of ${evacuateTarget} completed`
          : `Restore of chassis ${restoreTarget} completed`;
      pendingPlan = null;
      // Reload data after short delay to let OVN process
      setTimeout(() => {
        load();
        clearAction();
      }, 1500);
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to apply plan';
    } finally {
      actionLoading = false;
    }
  }

  async function confirmCancel() {
    if (pendingPlan) {
      try {
        await cancelPlan(pendingPlan.id);
      } catch {
        // ignore cancel errors
      }
    }
    clearAction();
  }
</script>

<PageHeader
  eyebrow="Visualize"
  title="HA Failover"
  description="HA Chassis Groups and gateway chassis failover chains."
>
  {#snippet actions()}
    <StatTiles
      tiles={[
        { label: 'HA Groups', value: groups.length },
        { label: 'Chassis Involved', value: totalChassisInvolved },
      ]}
    />
  {/snippet}
</PageHeader>

<DataState {loading} {error}>
  <!-- Summary + Search bar + Evacuate -->
  <div class="mb-4 flex flex-wrap items-center gap-2">
    <FilterInput
      bind:value={searchQuery}
      placeholder="filter by group name, chassis…"
      width="w-72"
    />

    {#if $writeEnabled && hasNbGroups && activeChassisInfo.length > 0}
      <button
        class="btn btn-outline btn-warning btn-sm font-mono normal-case"
        onclick={() => {
          showEvacuateDropdown = !showEvacuateDropdown;
          showRestoreDropdown = false;
          if (!showEvacuateDropdown) clearAction();
        }}
      >
        Evacuate Chassis
      </button>
    {/if}
    {#if $writeEnabled && drainedChassisInfo.length > 0}
      <button
        class="btn btn-outline btn-success btn-sm font-mono normal-case"
        onclick={() => {
          showRestoreDropdown = !showRestoreDropdown;
          showEvacuateDropdown = false;
          if (!showRestoreDropdown) clearAction();
        }}
      >
        Restore Chassis
      </button>
    {/if}
  </div>

    <!-- Shared chassis-selection panel (evacuate / restore) -->
    {#snippet selectionPanel(opts: SelectionPanelOpts)}
      <div class="mb-4 rounded border border-base-300 bg-base-100 p-4">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <div
              class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
            >
              {opts.title}
            </div>
            <div class="mt-1 font-prose text-xs text-base-content/55">
              {opts.description}
            </div>
          </div>
          <button
            class="btn btn-ghost btn-sm border-base-300"
            aria-label="Close"
            onclick={opts.onclose}
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>
        <div class="overflow-x-auto rounded border border-base-300">
          <table class="table table-xs font-mono">
            <thead>
              <tr>
                <th
                  class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                  >Chassis</th
                >
                <th
                  class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                  >Hostname</th
                >
                <th
                  class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                  >{opts.countHeader}</th
                >
                <th
                  class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                ></th>
              </tr>
            </thead>
            <tbody>
              {#each opts.rows as chassis (chassis.name)}
                <tr class="border-base-300/60 hover:bg-base-300/40">
                  <td class="text-xs">{chassis.name}</td>
                  <td class="text-xs text-base-content/70"
                    >{chassis.hostname || '—'}</td
                  >
                  <td>
                    <Badge
                      text="{chassis.count} group{chassis.count !== 1
                        ? 's'
                        : ''}"
                      variant={opts.countVariant(chassis.count)}
                    />
                  </td>
                  <td class="text-right">
                    <button
                      class="btn btn-xs font-mono normal-case {opts.actionClass}"
                      onclick={() => opts.onaction(chassis.name)}
                      disabled={actionLoading}
                    >
                      {opts.actionLabel}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/snippet}

    <!-- Evacuate chassis selection panel -->
    {#if showEvacuateDropdown && !evacuateTarget}
      {@render selectionPanel({
        title: 'Select chassis to evacuate',
        description:
          'Drains a chassis by setting its priority to 0 in all HA groups, letting OVN promote the next-highest-priority chassis.',
        countHeader: 'Active in',
        rows: activeChassisInfo.map((c) => ({
          name: c.name,
          hostname: c.hostname,
          count: c.activeInGroups.length,
        })),
        countVariant: (count) => (count > 2 ? 'warning' : 'neutral'),
        actionLabel: 'Evacuate',
        actionClass: 'btn-outline btn-warning',
        onaction: startEvacuate,
        onclose: () => {
          showEvacuateDropdown = false;
        },
      })}
    {/if}

    <!-- Restore chassis selection panel -->
    {#if showRestoreDropdown && !restoreTarget}
      {@render selectionPanel({
        title: 'Select chassis to restore',
        description:
          'Restores a previously drained chassis to its original priority (standby).',
        countHeader: 'Drained in',
        rows: drainedChassisInfo.map((c) => ({
          name: c.name,
          hostname: c.hostname,
          count: c.drainedInGroups.length,
        })),
        countVariant: () => 'error',
        actionLabel: 'Restore',
        actionClass: 'btn-outline btn-success',
        onaction: startRestore,
        onclose: () => {
          showRestoreDropdown = false;
          clearAction();
        },
      })}
    {/if}

    <!-- Evacuate / Failover / Restore confirmation panel -->
    {#if (failoverTarget || evacuateTarget || restoreTarget) && (pendingPlan || actionLoading || actionError || actionSuccess)}
      <div class="mb-4 rounded border-l-2 border-warning bg-warning/5 p-4">
        {#if actionSuccess}
          <div class="flex items-center gap-2 font-mono text-sm text-success">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-5 w-5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M5 13l4 4L19 7"
              />
            </svg>
            {actionSuccess}
          </div>
        {:else if actionLoading && !pendingPlan}
          <div class="flex items-center gap-2 font-mono text-sm">
            <span class="loading loading-spinner loading-sm"></span>
            Computing preview...
          </div>
        {:else if actionError && !pendingPlan}
          <div class="flex items-center justify-between">
            <span class="font-mono text-sm text-error">{actionError}</span>
            <button
              class="btn btn-ghost btn-sm border-base-300"
              onclick={clearAction}
            >
              Close
            </button>
          </div>
        {:else if pendingPlan}
          <div class="space-y-3">
            <!-- Header -->
            <div class="flex items-start justify-between gap-2">
              <div>
                {#if failoverTarget}
                  <div
                    class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
                  >
                    Failover: {failoverTarget.groupName}
                  </div>
                  <div class="mt-1 font-mono text-xs text-base-content/55">
                    {failoverTarget.activeChassisName} &rarr; {failoverTarget.targetChassis}
                  </div>
                {:else if evacuateTarget}
                  <div
                    class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
                  >
                    Evacuate: {evacuateTarget}
                  </div>
                  <div class="mt-1 font-mono text-xs text-base-content/55">
                    {pendingPlan.diffs.length} group(s) affected
                  </div>
                {:else if restoreTarget}
                  <div
                    class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
                  >
                    Restore: {restoreTarget}
                  </div>
                  <div class="mt-1 font-mono text-xs text-base-content/55">
                    {pendingPlan.diffs.length} group(s) affected
                  </div>
                {/if}
              </div>
              <Badge
                text="{pendingPlan.operations.length} operations"
                variant="warning"
              />
            </div>

            <!-- Diff table -->
            <div class="overflow-x-auto rounded border border-base-300">
              <table class="table table-xs font-mono">
                <thead>
                  <tr>
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Table</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >UUID</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Field</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Before</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >After</th
                    >
                  </tr>
                </thead>
                <tbody>
                  {#each pendingPlan.diffs as diff (diff.uuid)}
                    {#if diff.fields}
                      {#each diff.fields as field (field.field)}
                        <tr class="border-base-300/60">
                          <td class="text-xs">{diff.table}</td>
                          <td class="text-xs">
                            {diff.uuid}
                          </td>
                          <td class="text-xs">{field.field}</td>
                          <td class="text-error">{field.old_value}</td>
                          <td class="text-success">{field.new_value}</td>
                        </tr>
                      {/each}
                    {/if}
                  {/each}
                </tbody>
              </table>
            </div>

            <!-- Action error -->
            {#if actionError}
              <div class="font-mono text-sm text-error">{actionError}</div>
            {/if}

            <!-- Buttons -->
            <div class="flex items-center gap-2">
              <button
                class="btn btn-warning btn-sm font-mono normal-case"
                onclick={confirmApply}
                disabled={actionLoading}
              >
                {#if actionLoading}
                  <span class="loading loading-spinner loading-sm"></span>
                {/if}
                Apply
              </button>
              <button
                class="btn btn-ghost btn-sm border-base-300"
                onclick={confirmCancel}
                disabled={actionLoading}
              >
                Cancel
              </button>
            </div>
          </div>
        {/if}
      </div>
    {/if}

    {#if groups.length === 0}
      <div class="py-8 text-center">
        <span class="font-mono text-sm text-base-content/40"
          ><span class="text-base-content/30">//</span> no HA Chassis Groups found</span
        >
      </div>
    {:else if filteredGroups.length === 0}
      <div class="py-8 text-center">
        <span class="font-mono text-sm text-base-content/40"
          ><span class="text-base-content/30">//</span> no groups match the filter</span
        >
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-4">
        {#each filteredGroups as group (group.uuid)}
          <div class="rounded border border-base-300 bg-base-100 p-4">
            <div>
              <!-- Group header -->
              <div
                class="mb-3 flex flex-wrap items-start justify-between gap-2"
              >
                <div>
                  <h2
                    class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
                  >
                    {shortName(group.name)}
                  </h2>
                  {#if group.crPortName}
                    <p class="mt-0.5 font-mono text-xs text-base-content/50">
                      CR port: {group.crPortName}
                    </p>
                  {/if}
                </div>
                <div class="flex items-center gap-2">
                  <Badge
                    text="{group.chassisChain.length} chassis"
                    variant="neutral"
                    outline
                  />
                  {#if group.activeChassis}
                    <Badge text="has active" variant="success" />
                  {:else}
                    <Badge text="no active" variant="warning" />
                  {/if}
                  {#if $writeEnabled && group.nbGroupName && group.activeChassis && group.chassisChain.length > 1}
                    {@const activeEntry = group.chassisChain.find(
                      (c) => c.isActive,
                    )}
                    {@const standbyEntry = group.chassisChain.find(
                      (c) => !c.isActive,
                    )}
                    {#if activeEntry && standbyEntry}
                      <button
                        class="btn btn-square btn-outline btn-warning btn-xs font-mono"
                        aria-label="Failover to {standbyEntry.chassisName}"
                        title="Failover to {standbyEntry.chassisName}"
                        onclick={() =>
                          startFailover(
                            group.nbGroupName!,
                            standbyEntry.chassisName,
                            activeEntry.chassisName,
                          )}
                        disabled={actionLoading}
                      >
                        <svg
                          xmlns="http://www.w3.org/2000/svg"
                          class="h-3.5 w-3.5"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"
                          />
                        </svg>
                      </button>
                    {/if}
                  {/if}
                </div>
              </div>

              <!-- Chassis failover chain -->
              {#if group.chassisChain.length === 0}
                <div class="py-2">
                  <span class="font-mono text-xs text-base-content/40"
                    ><span class="text-base-content/30">//</span> no chassis entries
                    in this group</span
                  >
                </div>
              {:else}
                <div class="flex flex-wrap items-center gap-0">
                  {#each group.chassisChain as entry, idx (entry.uuid)}
                    <!-- Chassis box -->
                    <div
                      class="relative flex min-w-[140px] flex-col rounded border-l-2 px-3 py-2 {entry.isActive
                        ? 'border-success bg-success/5'
                        : 'border-base-300 bg-base-200/50'}"
                    >
                      <!-- Priority badge -->
                      <div class="mb-1 flex items-center justify-between gap-2">
                        <Badge
                          text="P{entry.priority}"
                          variant={entry.isActive ? 'success' : 'ghost'}
                        />
                        {#if entry.isActive}
                          <Badge text="ACTIVE" variant="success" />
                        {:else if entry.isDrained}
                          <Badge text="DRAINED" variant="error" />
                        {:else if idx === 0 && !group.activeChassis}
                          <Badge text="STANDBY" variant="warning" />
                        {:else}
                          <Badge text="STANDBY" variant="ghost" />
                        {/if}
                      </div>
                      <!-- Chassis name -->
                      <div
                        class="font-mono text-sm font-medium"
                        title={entry.chassisName}
                      >
                        {shortName(entry.chassisName)}
                      </div>
                      {#if entry.hostname}
                        <div
                          class="font-mono text-xs text-base-content/50"
                          title={entry.hostname}
                        >
                          {entry.hostname}
                        </div>
                      {/if}
                    </div>

                    <!-- Arrow connector between chassis boxes -->
                    {#if idx < group.chassisChain.length - 1}
                      <div class="flex items-center px-1 text-base-content/30">
                        <svg
                          xmlns="http://www.w3.org/2000/svg"
                          class="h-5 w-5"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M9 5l7 7-7 7"
                          />
                        </svg>
                      </div>
                    {/if}
                  {/each}
                </div>

                <!-- Legend for this card -->
                <div class="mt-2 font-prose text-xs text-base-content/40">
                  Ordered by priority (highest first). Highest priority with
                  bound CR port = active gateway.
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </DataState>
