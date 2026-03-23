<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { get as apiGet } from '../lib/api';
  import { writeEnabled } from '../lib/capabilitiesStore';
  import {
    previewOperations,
    applyPlan,
    cancelPlan,
    type Plan,
    type WriteOperation,
  } from '../lib/writeApi';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import PlanDiffView from '../components/write/PlanDiffView.svelte';

  interface StaleEntry {
    type: string;
    severity: string;
    uuid: string;
    table: string;
    message: string;
    details?: Record<string, unknown>;
    age_seconds?: number;
  }

  interface StaleResult {
    total: number;
    stale_mac_bindings: number;
    orphaned_fdb: number;
    orphaned_port_bindings: number;
    entries: StaleEntry[];
  }

  let data: StaleResult | null = $state(null);
  let loading = $state(true);
  let error = $state('');
  let typeFilter = $state<'all' | 'mac_binding' | 'fdb' | 'port_binding'>(
    'all',
  );

  // Selection state
  let selected = new SvelteSet<string>();

  // Delete flow state
  let deleteStep = $state<
    'idle' | 'preview' | 'confirming' | 'applying' | 'done' | 'error'
  >('idle');
  let plan = $state<Plan | null>(null);
  let deleteError = $state('');
  let actor = $state('');

  function setTypeFilter(val: typeof typeFilter) {
    typeFilter = val;
    selected.clear();
  }

  let filtered = $derived(
    (data?.entries ?? []).filter(
      (e) => typeFilter === 'all' || e.type === typeFilter,
    ),
  );

  let allFilteredSelected = $derived(
    filtered.length > 0 && filtered.every((e) => selected.has(e.uuid)),
  );

  function formatAge(seconds?: number): string {
    if (!seconds) return '';
    const hours = Math.floor(seconds / 3600);
    if (hours > 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
    return `${hours}h`;
  }

  function toggleSelect(uuid: string) {
    if (selected.has(uuid)) {
      selected.delete(uuid);
    } else {
      selected.add(uuid);
    }
  }

  function toggleSelectAll() {
    if (allFilteredSelected) {
      for (const e of filtered) selected.delete(e.uuid);
    } else {
      for (const e of filtered) selected.add(e.uuid);
    }
  }

  function selectedEntries(): StaleEntry[] {
    return filtered.filter((e) => selected.has(e.uuid));
  }

  async function startDelete() {
    const entries = selectedEntries();
    if (entries.length === 0) return;

    deleteStep = 'preview';
    deleteError = '';
    plan = null;

    const operations: WriteOperation[] = entries.map((e) => ({
      action: 'delete' as const,
      table: e.table,
      uuid: e.uuid,
      reason: `Stale entry cleanup: ${e.message}`,
    }));

    try {
      plan = await previewOperations(operations, 'Stale entry cleanup');
      deleteStep = 'confirming';
    } catch (e) {
      deleteError = e instanceof Error ? e.message : 'Preview failed';
      deleteStep = 'error';
    }
  }

  async function confirmDelete() {
    if (!plan) return;
    deleteStep = 'applying';
    try {
      await applyPlan(plan.id, plan.apply_token, actor || undefined);
      deleteStep = 'done';
      selected.clear();
      // Reload the data
      await load();
    } catch (e) {
      deleteError = e instanceof Error ? e.message : 'Apply failed';
      deleteStep = 'error';
    }
  }

  async function cancelDelete() {
    if (plan && deleteStep === 'confirming') {
      try {
        await cancelPlan(plan.id);
      } catch {
        // ignore cancel errors
      }
    }
    resetDeleteFlow();
  }

  function resetDeleteFlow() {
    deleteStep = 'idle';
    plan = null;
    deleteError = '';
    actor = '';
  }

  async function load() {
    loading = true;
    error = '';
    try {
      data = await apiGet<StaleResult>('/api/v1/debug/stale-entries');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  // Track plan expiration silently — only show error if user tries to confirm after expiry
  let expired = $state(false);

  $effect(() => {
    if (!plan?.expires_at) {
      expired = false;
      return;
    }
    const check = () => {
      expired = Date.now() >= new Date(plan!.expires_at).getTime();
    };
    check();
    const interval = setInterval(check, 1000);
    return () => clearInterval(interval);
  });

  onMount(() => load());
</script>

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Stale Entries</h1>
    <p class="text-sm text-base-content/60">
      Aged MAC bindings, orphaned FDB entries, and port bindings without NB
      counterparts
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if loading}
    <LoadingSpinner />
  {:else if data}
    <div class="stats mb-4 w-full border border-base-300 bg-base-100 shadow-sm">
      <div class="stat">
        <div class="stat-title">Total</div>
        <div class="stat-value text-lg">{data.total}</div>
      </div>
      <div class="stat">
        <div class="stat-title">Stale MAC</div>
        <div class="stat-value text-lg text-warning">
          {data.stale_mac_bindings}
        </div>
      </div>
      <div class="stat">
        <div class="stat-title">Orphaned FDB</div>
        <div class="stat-value text-lg text-warning">{data.orphaned_fdb}</div>
      </div>
      <div class="stat">
        <div class="stat-title">Orphaned Ports</div>
        <div class="stat-value text-lg text-error">
          {data.orphaned_port_bindings}
        </div>
      </div>
    </div>

    <div class="mb-4 flex items-center gap-3">
      <div class="join">
        {#each [['all', 'All'], ['mac_binding', 'MAC'], ['fdb', 'FDB'], ['port_binding', 'Ports']] as [val, label] (val)}
          <button
            class="btn join-item btn-xs {typeFilter === val
              ? 'btn-active'
              : ''}"
            onclick={() => setTypeFilter(val as typeof typeFilter)}
            >{label}</button
          >
        {/each}
      </div>
      <span class="text-sm text-base-content/50">{filtered.length} entries</span
      >

      {#if $writeEnabled && selected.size > 0 && deleteStep === 'idle'}
        <button class="btn btn-error btn-xs ml-auto" onclick={startDelete}>
          Delete {selected.size} selected
        </button>
      {/if}
    </div>

    <!-- Delete confirmation modal -->
    {#if deleteStep !== 'idle'}
      <div
        class="mb-4 rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm"
      >
        {#if deleteStep === 'preview'}
          <div class="flex items-center gap-2">
            <span class="loading loading-spinner loading-sm"></span>
            <span class="text-sm">Previewing changes...</span>
          </div>
        {:else if deleteStep === 'confirming' && plan}
          <div class="flex flex-col gap-3">
            <h3 class="text-sm font-semibold">
              Confirm Deletion &mdash; {plan.operations.length} entr{plan
                .operations.length === 1
                ? 'y'
                : 'ies'}
            </h3>

            <PlanDiffView diffs={plan.diffs} />

            <div
              class="flex flex-wrap items-end gap-3 border-t border-base-300 pt-3"
            >
              <div class="form-control">
                <label class="label py-0.5" for="stale-actor">
                  <span class="label-text text-xs">Actor (optional)</span>
                </label>
                <input
                  id="stale-actor"
                  type="text"
                  class="input input-sm input-bordered w-48"
                  placeholder="your-name"
                  bind:value={actor}
                />
              </div>
              <button
                class="btn btn-error btn-sm"
                disabled={expired}
                onclick={confirmDelete}
              >
                Confirm Delete
              </button>
              <button class="btn btn-ghost btn-sm" onclick={cancelDelete}>
                Cancel
              </button>
            </div>

            {#if expired}
              <div role="alert" class="alert alert-error py-2 text-xs">
                Plan has expired. Cancel and try again.
              </div>
            {/if}
          </div>
        {:else if deleteStep === 'applying'}
          <div class="flex items-center gap-2">
            <span class="loading loading-spinner loading-sm"></span>
            <span class="text-sm">Deleting entries...</span>
          </div>
        {:else if deleteStep === 'done'}
          <div role="alert" class="alert alert-success py-2">
            <span class="text-sm">Entries deleted successfully.</span>
            <button class="btn btn-ghost btn-xs" onclick={resetDeleteFlow}>
              Dismiss
            </button>
          </div>
        {:else if deleteStep === 'error'}
          <div role="alert" class="alert alert-error py-2">
            <span class="text-sm">Error: {deleteError}</span>
            <button class="btn btn-ghost btn-xs" onclick={resetDeleteFlow}>
              Dismiss
            </button>
          </div>
        {/if}
      </div>
    {/if}

    <div class="overflow-x-auto">
      <table class="table table-sm">
        <thead>
          <tr>
            {#if $writeEnabled}
              <th class="w-8">
                <input
                  type="checkbox"
                  class="checkbox checkbox-xs"
                  checked={allFilteredSelected}
                  onchange={toggleSelectAll}
                  disabled={deleteStep !== 'idle'}
                />
              </th>
            {/if}
            <th>Severity</th>
            <th>Type</th>
            <th>Table</th>
            <th>Message</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as entry (entry.uuid)}
            <tr class={selected.has(entry.uuid) ? 'bg-base-200' : ''}>
              {#if $writeEnabled}
                <td>
                  <input
                    type="checkbox"
                    class="checkbox checkbox-xs"
                    checked={selected.has(entry.uuid)}
                    onchange={() => toggleSelect(entry.uuid)}
                    disabled={deleteStep !== 'idle'}
                  />
                </td>
              {/if}
              <td
                ><span
                  class="badge badge-sm {entry.severity === 'error'
                    ? 'badge-error'
                    : 'badge-warning'}">{entry.severity}</span
                ></td
              >
              <td
                ><span class="badge badge-ghost badge-sm">{entry.type}</span
                ></td
              >
              <td class="font-mono text-xs">{entry.table}</td>
              <td class="text-sm">{entry.message}</td>
              <td class="text-xs text-base-content/50"
                >{formatAge(entry.age_seconds)}</td
              >
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if filtered.length === 0}
      <div class="py-8 text-center text-sm text-base-content/40">
        {data.total === 0
          ? 'No stale entries detected'
          : 'No entries match the current filter'}
      </div>
    {/if}
  {/if}
</div>
