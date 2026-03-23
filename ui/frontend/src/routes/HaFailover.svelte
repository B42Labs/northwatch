<script lang="ts">
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { writeEnabled } from '../lib/capabilitiesStore';
  import {
    requestFailover,
    requestEvacuate,
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
  let showEvacuateDropdown = $state(false);
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

            return {
              uuid: hc._uuid,
              chassisUuid: hc.chassis,
              chassisName: chassisRecord?.name ?? hc.chassis ?? 'unknown',
              hostname: chassisRecord?.hostname ?? '',
              priority: hc.priority,
              isActive: hc.chassis === activeChassis && activeChassis !== null,
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
    showEvacuateDropdown = false;
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

  async function confirmApply() {
    if (!pendingPlan) return;
    actionLoading = true;
    actionError = '';
    try {
      await applyPlan(pendingPlan.id, pendingPlan.apply_token, 'northwatch-ui');
      actionSuccess = failoverTarget
        ? `Failover completed: ${failoverTarget.activeChassisName} \u2192 ${failoverTarget.targetChassis}`
        : `Evacuation of ${evacuateTarget} completed`;
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

<div>
  <!-- Header -->
  <div class="mb-4">
    <h1 class="text-xl font-bold">HA Failover</h1>
    <p class="text-sm text-base-content/60">
      HA Chassis Groups and gateway chassis failover chains
    </p>
  </div>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else}
    <!-- Summary + Search bar + Evacuate -->
    <div class="mb-4 flex flex-wrap items-center gap-4">
      <div>
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Filter by group name, chassis..."
          class="input input-sm input-bordered w-72"
        />
      </div>

      {#if $writeEnabled && hasNbGroups && activeChassisInfo.length > 0}
        <button
          class="btn btn-outline btn-warning btn-sm"
          onclick={() => {
            showEvacuateDropdown = !showEvacuateDropdown;
            if (!showEvacuateDropdown) clearAction();
          }}
        >
          Evacuate Chassis
        </button>
      {/if}

      <div
        class="stats stats-horizontal ml-auto border border-base-300 bg-base-100 shadow-sm"
      >
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">HA Groups</div>
          <div class="stat-value text-lg">{groups.length}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Chassis Involved</div>
          <div class="stat-value text-lg">{totalChassisInvolved}</div>
        </div>
      </div>
    </div>

    <!-- Evacuate chassis selection panel -->
    {#if showEvacuateDropdown && !evacuateTarget}
      <div
        class="mb-4 rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm"
      >
        <div class="mb-3 flex items-center justify-between">
          <div>
            <div class="text-sm font-semibold">Select chassis to evacuate</div>
            <div class="text-xs text-base-content/60">
              Evacuating a chassis swaps priorities so it is no longer active in
              any HA group.
            </div>
          </div>
          <button
            class="btn btn-ghost btn-sm"
            aria-label="Close"
            onclick={() => {
              showEvacuateDropdown = false;
            }}
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
        <div class="overflow-x-auto">
          <table class="table table-sm">
            <thead>
              <tr>
                <th>Chassis</th>
                <th>Hostname</th>
                <th>Active in</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {#each activeChassisInfo as chassis (chassis.name)}
                <tr class="hover">
                  <td class="font-mono text-sm">{chassis.name}</td>
                  <td class="text-sm text-base-content/70"
                    >{chassis.hostname || '—'}</td
                  >
                  <td>
                    <span
                      class="badge badge-sm"
                      class:badge-warning={chassis.activeInGroups.length > 2}
                    >
                      {chassis.activeInGroups.length} group{chassis
                        .activeInGroups.length !== 1
                        ? 's'
                        : ''}
                    </span>
                  </td>
                  <td class="text-right">
                    <button
                      class="btn btn-outline btn-warning btn-xs"
                      onclick={() => startEvacuate(chassis.name)}
                      disabled={actionLoading}
                    >
                      Evacuate
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}

    <!-- Evacuate / Failover confirmation panel -->
    {#if (failoverTarget || evacuateTarget) && (pendingPlan || actionLoading || actionError || actionSuccess)}
      <div class="mb-4 rounded-lg border-2 border-warning bg-warning/5 p-4">
        {#if actionSuccess}
          <div class="flex items-center gap-2 text-sm text-success">
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
          <div class="flex items-center gap-2 text-sm">
            <span class="loading loading-spinner loading-sm"></span>
            Computing preview...
          </div>
        {:else if actionError && !pendingPlan}
          <div class="flex items-center justify-between">
            <span class="text-sm text-error">{actionError}</span>
            <button class="btn btn-ghost btn-sm" onclick={clearAction}>
              Close
            </button>
          </div>
        {:else if pendingPlan}
          <div class="space-y-3">
            <!-- Header -->
            <div class="flex items-start justify-between">
              <div>
                {#if failoverTarget}
                  <div class="text-sm font-semibold">
                    Failover: {failoverTarget.groupName}
                  </div>
                  <div class="text-xs text-base-content/60">
                    {failoverTarget.activeChassisName} &rarr; {failoverTarget.targetChassis}
                  </div>
                {:else if evacuateTarget}
                  <div class="text-sm font-semibold">
                    Evacuate: {evacuateTarget}
                  </div>
                  <div class="text-xs text-base-content/60">
                    {pendingPlan.diffs.length} group(s) affected
                  </div>
                {/if}
              </div>
              <span class="badge badge-warning badge-sm">
                {pendingPlan.operations.length} operations
              </span>
            </div>

            <!-- Diff table -->
            <div class="overflow-x-auto">
              <table class="table table-xs">
                <thead>
                  <tr>
                    <th>Table</th>
                    <th>UUID</th>
                    <th>Field</th>
                    <th>Before</th>
                    <th>After</th>
                  </tr>
                </thead>
                <tbody>
                  {#each pendingPlan.diffs as diff (diff.uuid)}
                    {#if diff.fields}
                      {#each diff.fields as field (field.field)}
                        <tr>
                          <td class="font-mono text-xs">{diff.table}</td>
                          <td class="font-mono text-xs">
                            {diff.uuid}
                          </td>
                          <td class="font-mono text-xs">{field.field}</td>
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
              <div class="text-sm text-error">{actionError}</div>
            {/if}

            <!-- Buttons -->
            <div class="flex items-center gap-2">
              <button
                class="btn btn-warning btn-sm"
                onclick={confirmApply}
                disabled={actionLoading}
              >
                {#if actionLoading}
                  <span class="loading loading-spinner loading-sm"></span>
                {/if}
                Apply
              </button>
              <button
                class="btn btn-ghost btn-sm"
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
      <div class="py-8 text-center text-base-content/50">
        No HA Chassis Groups found
      </div>
    {:else if filteredGroups.length === 0}
      <div class="py-8 text-center text-base-content/50">
        No groups match the filter
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-4">
        {#each filteredGroups as group (group.uuid)}
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body p-4">
              <!-- Group header -->
              <div
                class="mb-3 flex flex-wrap items-start justify-between gap-2"
              >
                <div>
                  <h2 class="card-title text-sm font-semibold">
                    {shortName(group.name)}
                  </h2>
                  {#if group.crPortName}
                    <p class="mt-0.5 text-xs text-base-content/50">
                      CR port: {group.crPortName}
                    </p>
                  {/if}
                </div>
                <div class="flex items-center gap-2">
                  <span class="badge badge-outline badge-sm">
                    {group.chassisChain.length} chassis
                  </span>
                  {#if group.activeChassis}
                    <span class="badge badge-success badge-sm">has active</span>
                  {:else}
                    <span class="badge badge-warning badge-sm">no active</span>
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
                        class="btn btn-square btn-outline btn-warning btn-xs"
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
                <div class="py-2 text-xs text-base-content/40">
                  No chassis entries in this group
                </div>
              {:else}
                <div class="flex flex-wrap items-center gap-0">
                  {#each group.chassisChain as entry, idx (entry.uuid)}
                    <!-- Chassis box -->
                    <div
                      class="relative flex min-w-[140px] flex-col rounded-lg border-2 px-3 py-2 {entry.isActive
                        ? 'border-success bg-success/5'
                        : 'border-base-300 bg-base-200/50'}"
                    >
                      <!-- Priority badge -->
                      <div class="mb-1 flex items-center justify-between gap-2">
                        <span
                          class="badge badge-sm font-mono {entry.isActive
                            ? 'badge-success'
                            : 'badge-ghost'}"
                        >
                          P{entry.priority}
                        </span>
                        {#if entry.isActive}
                          <span class="badge badge-success badge-sm"
                            >ACTIVE</span
                          >
                        {:else if idx === 0 && !group.activeChassis}
                          <span class="badge badge-warning badge-sm"
                            >STANDBY</span
                          >
                        {:else}
                          <span class="badge badge-ghost badge-sm">STANDBY</span
                          >
                        {/if}
                      </div>
                      <!-- Chassis name -->
                      <div
                        class="text-sm font-medium"
                        title={entry.chassisName}
                      >
                        {shortName(entry.chassisName)}
                      </div>
                      {#if entry.hostname}
                        <div
                          class="text-xs text-base-content/50"
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
                <div class="mt-2 text-xs text-base-content/40">
                  Ordered by priority (highest first). Highest priority with
                  bound CR port = active gateway.
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
