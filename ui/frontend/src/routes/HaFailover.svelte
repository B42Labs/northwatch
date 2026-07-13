<script lang="ts">
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import GatewayHealthPanel from '../components/gateway/GatewayHealthPanel.svelte';
  import { writeEnabled } from '../lib/capabilitiesStore';
  import {
    requestFailover,
    requestEvacuate,
    requestRestore,
    applyPlan,
    cancelPlan,
    type Plan,
  } from '../lib/writeApi';
  import { get } from '../lib/api';
  import {
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
    type ResolvedGroup,
    type DrainedChassisInfo,
  } from '../lib/haFailover';

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
  let drainedChassisInfo: DrainedChassisInfo[] = $state([]);

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

  // --- Deriveds (thin wrappers over the pure lib/haFailover helpers) ---

  // Active chassis with context for the evacuate panel.
  let activeChassisInfo = $derived(computeActiveChassisInfo(groups));

  // Filtered groups for the search bar.
  let filteredGroups = $derived(filterGroups(groups, searchQuery));

  // --- Load data ---

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
        get<SbHaChassisGroup[]>('/api/v1/sb/ha-chassis-groups'),
        get<SbHaChassisEntry[]>('/api/v1/sb/ha-chassis'),
        get<ChassisRecord[]>('/api/v1/sb/chassis'),
        get<PortBindingRecord[]>('/api/v1/sb/port-bindings'),
        get<NbHaChassisGroup[]>('/api/v1/nb/ha-chassis-groups'),
        get<NbHaChassisEntry[]>('/api/v1/nb/ha-chassis'),
        get<NbGatewayChassisEntry[]>('/api/v1/nb/gateway-chassis'),
        get<NbLogicalRouterPort[]>('/api/v1/nb/logical-router-ports'),
      ]);

      const resolved = resolveHaGroups({
        sbHaGroups,
        sbHaChassis,
        sbChassisList,
        sbPortBindings,
        nbHaGroups,
        nbHaChassis,
        nbGwChassis,
        nbLrps,
      });

      groups = resolved.groups;
      totalChassisInvolved = resolved.totalChassisInvolved;
      drainedChassisInfo = resolved.drainedChassisInfo;
      hasNbGroups = resolved.hasNbGroups;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load HA data';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load();
  });

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
      await applyPlan(pendingPlan.id, pendingPlan.apply_token);
      actionSuccess = failoverTarget
        ? `Failover completed: ${failoverTarget.activeChassisName} → ${failoverTarget.targetChassis}`
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

<!-- Shared chassis-selection panel (evacuate / restore). Declared at the top
     level so it is a local snippet rendered via {@render} below, not a prop
     passed to an enclosing component. -->
{#snippet selectionPanel(opts: SelectionPanelOpts)}
  <div class="mb-4 rounded border border-base-300 bg-base-100 p-4">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <div
          class="font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
        >
          {opts.title}
        </div>
        <div class="font-prose mt-1 text-xs text-base-content/55">
          {opts.description}
        </div>
      </div>
      <button
        class="btn border-base-300 btn-ghost btn-sm"
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
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Chassis</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Hostname</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >{opts.countHeader}</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
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
                  text="{chassis.count} group{chassis.count !== 1 ? 's' : ''}"
                  variant={opts.countVariant(chassis.count)}
                />
              </td>
              <td class="text-right">
                <button
                  class="btn font-mono normal-case btn-xs {opts.actionClass}"
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

<!-- Read-only detection: highest-priority-alive (desired) vs Port_Binding.chassis
     (actual) for every chassisredirect port. Works regardless of write mode. -->
<GatewayHealthPanel />

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
        class="btn btn-outline font-mono normal-case btn-sm btn-warning"
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
        class="btn btn-outline font-mono normal-case btn-sm btn-success"
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
          <span class="loading loading-sm loading-spinner"></span>
          Computing preview...
        </div>
      {:else if actionError && !pendingPlan}
        <div class="flex items-center justify-between">
          <span class="font-mono text-sm text-error">{actionError}</span>
          <button
            class="btn border-base-300 btn-ghost btn-sm"
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
                  class="font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
                >
                  Failover: {failoverTarget.groupName}
                </div>
                <div class="mt-1 font-mono text-xs text-base-content/55">
                  {failoverTarget.activeChassisName} &rarr; {failoverTarget.targetChassis}
                </div>
              {:else if evacuateTarget}
                <div
                  class="font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
                >
                  Evacuate: {evacuateTarget}
                </div>
                <div class="mt-1 font-mono text-xs text-base-content/55">
                  {pendingPlan.diffs.length} group(s) affected
                </div>
              {:else if restoreTarget}
                <div
                  class="font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
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
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                    >Table</th
                  >
                  <th
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                    >UUID</th
                  >
                  <th
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                    >Field</th
                  >
                  <th
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                    >Before</th
                  >
                  <th
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
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
              class="btn font-mono normal-case btn-sm btn-warning"
              onclick={confirmApply}
              disabled={actionLoading}
            >
              {#if actionLoading}
                <span class="loading loading-sm loading-spinner"></span>
              {/if}
              Apply
            </button>
            <button
              class="btn border-base-300 btn-ghost btn-sm"
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
            <div class="mb-3 flex flex-wrap items-start justify-between gap-2">
              <div>
                <h2
                  class="font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
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
                      class="btn btn-square btn-outline font-mono btn-warning btn-xs"
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
                  {@const status = entryStatus(
                    entry,
                    idx,
                    !!group.activeChassis,
                  )}
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
                      <Badge text={status.label} variant={status.variant} />
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
              <div class="font-prose mt-2 text-xs text-base-content/40">
                Ordered by priority (highest first). Highest priority with bound
                CR port = active gateway.
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</DataState>
