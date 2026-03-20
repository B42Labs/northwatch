<script lang="ts">
  import { onMount } from 'svelte';
  import {
    getFlowDiff,
    listDatapathBindings,
    type FlowDiffResponse,
  } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { subscribeToTable } from '../lib/eventStore';

  let datapaths: Record<string, unknown>[] = $state([]);
  let datapathsLoading = $state(true);
  let selectedDatapath = $state('');
  let timeRange = $state<5 | 15 | 30 | 0>(30);
  let autoRefresh = $state(false);
  let diffData: FlowDiffResponse | null = $state(null);
  let loading = $state(false);
  let error = $state('');
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;

  interface DatapathOption {
    uuid: string;
    name: string;
    type: string;
  }

  let datapathOptions = $derived<DatapathOption[]>(
    datapaths
      .map((dp) => {
        const uuid = dp._uuid as string;
        const extIds = (dp.external_ids ?? {}) as Record<string, string>;
        const name = extIds['name'] || uuid.slice(0, 8);
        const type = extIds['logical-switch']
          ? 'switch'
          : extIds['logical-router']
            ? 'router'
            : 'unknown';
        return { uuid, name, type };
      })
      .sort((a, b) => a.name.localeCompare(b.name)),
  );

  let sinceTimestamp = $derived(
    timeRange > 0 ? Date.now() - timeRange * 60 * 1000 : 0,
  );

  let inserts = $derived(
    diffData?.changes.filter((c) => c.type === 'insert').length ?? 0,
  );
  let updates = $derived(
    diffData?.changes.filter((c) => c.type === 'update').length ?? 0,
  );
  let deletes = $derived(
    diffData?.changes.filter((c) => c.type === 'delete').length ?? 0,
  );

  function changeColor(type: string): string {
    switch (type) {
      case 'insert':
        return 'border-l-success';
      case 'delete':
        return 'border-l-error';
      case 'update':
        return 'border-l-warning';
      default:
        return 'border-l-base-300';
    }
  }

  function changeBadge(type: string): string {
    switch (type) {
      case 'insert':
        return 'badge-success';
      case 'delete':
        return 'badge-error';
      case 'update':
        return 'badge-warning';
      default:
        return 'badge-ghost';
    }
  }

  function formatTime(ts: number): string {
    return new Date(ts).toLocaleTimeString();
  }

  function formatTimeDelta(ts: number): string {
    const delta = Math.floor((Date.now() - ts) / 1000);
    if (delta < 60) return `${delta}s ago`;
    if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
    return `${Math.floor(delta / 3600)}h ago`;
  }

  async function loadDatapaths() {
    try {
      datapaths = await listDatapathBindings();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load datapaths';
    } finally {
      datapathsLoading = false;
    }
  }

  async function loadDiff() {
    loading = true;
    error = '';
    try {
      diffData = await getFlowDiff({
        datapath: selectedDatapath || undefined,
        since: sinceTimestamp || undefined,
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load flow diff';
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadDatapaths();
  });

  $effect(() => {
    // Track reactive dependencies so loadDiff re-runs when filters change
    void selectedDatapath;
    void timeRange;
    loadDiff();
  });

  // Auto-refresh on WS notification
  $effect(() => {
    if (!autoRefresh) return;
    const unsubscribe = subscribeToTable('sb', 'Logical_Flow', () => {
      if (refreshTimer) clearTimeout(refreshTimer);
      refreshTimer = setTimeout(() => {
        if (!loading) loadDiff();
      }, 500);
    });
    return () => {
      unsubscribe();
      if (refreshTimer) clearTimeout(refreshTimer);
    };
  });
</script>

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Flow Diff</h1>
    <p class="text-sm text-base-content/60">
      Track LogicalFlow changes over time — inserts, updates, and deletes
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {/if}

  {#if datapathsLoading}
    <LoadingSpinner />
  {:else}
    <!-- Controls -->
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <select
        bind:value={selectedDatapath}
        class="select select-bordered select-sm w-64"
      >
        <option value="">All datapaths</option>
        {#each datapathOptions as dp (dp.uuid)}
          <option value={dp.uuid}>[{dp.type}] {dp.name}</option>
        {/each}
      </select>

      <div class="join">
        <button
          class="btn join-item btn-xs {timeRange === 5 ? 'btn-active' : ''}"
          onclick={() => (timeRange = 5)}>5 min</button
        >
        <button
          class="btn join-item btn-xs {timeRange === 15 ? 'btn-active' : ''}"
          onclick={() => (timeRange = 15)}>15 min</button
        >
        <button
          class="btn join-item btn-xs {timeRange === 30 ? 'btn-active' : ''}"
          onclick={() => (timeRange = 30)}>30 min</button
        >
        <button
          class="btn join-item btn-xs {timeRange === 0 ? 'btn-active' : ''}"
          onclick={() => (timeRange = 0)}>All</button
        >
      </div>

      <label class="label cursor-pointer gap-2">
        <span class="label-text text-xs">Auto-refresh</span>
        <input
          type="checkbox"
          bind:checked={autoRefresh}
          class="toggle toggle-primary toggle-xs"
        />
      </label>

      <button class="btn btn-ghost btn-xs" onclick={loadDiff}>Refresh</button>
    </div>

    {#if loading && !diffData}
      <LoadingSpinner />
    {:else if diffData}
      <!-- Stats bar -->
      <div
        class="stats mb-4 w-full border border-base-300 bg-base-100 shadow-sm"
      >
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Total Changes</div>
          <div class="stat-value text-lg">{diffData.count}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Inserts</div>
          <div class="stat-value text-lg text-success">{inserts}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Updates</div>
          <div class="stat-value text-lg text-warning">{updates}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Deletes</div>
          <div class="stat-value text-lg text-error">{deletes}</div>
        </div>
      </div>

      <!-- Timeline -->
      {#if diffData.changes.length === 0}
        <div class="py-8 text-center text-sm text-base-content/40">
          No flow changes in the selected time range
        </div>
      {:else}
        <div class="flex flex-col gap-2">
          {#each diffData.changes as change (change.uuid + change.timestamp)}
            <div
              class="rounded-lg border-l-4 bg-base-100 px-4 py-3 shadow-sm {changeColor(
                change.type,
              )}"
            >
              <div class="flex items-center gap-2">
                <span class="badge badge-sm {changeBadge(change.type)}"
                  >{change.type}</span
                >
                <span class="font-mono text-xs text-base-content/50"
                  >{change.uuid.slice(0, 12)}</span
                >
                <span class="ml-auto text-xs text-base-content/40"
                  >{formatTime(change.timestamp)} ({formatTimeDelta(
                    change.timestamp,
                  )})</span
                >
              </div>

              {#if change.type === 'update' && change.old_row && change.new_row}
                <div class="mt-2 grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <div class="mb-0.5 font-semibold text-base-content/50">
                      Before
                    </div>
                    {#if change.old_row.match}
                      <div>
                        <span class="text-base-content/50">match:</span>
                        <span class="font-mono">{change.old_row.match}</span>
                      </div>
                    {/if}
                    {#if change.old_row.actions}
                      <div>
                        <span class="text-base-content/50">actions:</span>
                        <span class="font-mono">{change.old_row.actions}</span>
                      </div>
                    {/if}
                  </div>
                  <div>
                    <div class="mb-0.5 font-semibold text-base-content/50">
                      After
                    </div>
                    {#if change.new_row.match}
                      <div>
                        <span class="text-base-content/50">match:</span>
                        <span class="font-mono">{change.new_row.match}</span>
                      </div>
                    {/if}
                    {#if change.new_row.actions}
                      <div>
                        <span class="text-base-content/50">actions:</span>
                        <span class="font-mono">{change.new_row.actions}</span>
                      </div>
                    {/if}
                  </div>
                </div>
              {:else if change.new_row}
                <div class="mt-1 text-xs">
                  {#if change.new_row.match}
                    <span class="text-base-content/50">match:</span>
                    <span class="font-mono">{change.new_row.match}</span>
                  {/if}
                  {#if change.new_row.actions}
                    <span class="ml-2 text-base-content/50">actions:</span>
                    <span class="font-mono">{change.new_row.actions}</span>
                  {/if}
                </div>
              {:else if change.old_row}
                <div class="mt-1 text-xs line-through opacity-60">
                  {#if change.old_row.match}
                    <span class="text-base-content/50">match:</span>
                    <span class="font-mono">{change.old_row.match}</span>
                  {/if}
                  {#if change.old_row.actions}
                    <span class="ml-2 text-base-content/50">actions:</span>
                    <span class="font-mono">{change.old_row.actions}</span>
                  {/if}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  {/if}
</div>
