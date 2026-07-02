<script lang="ts">
  import { onMount } from 'svelte';
  import {
    getFlowDiff,
    listDatapathBindings,
    type FlowDiffResponse,
  } from '../lib/api';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import { actionVariant, actionGlyph } from '../lib/status';
  import { subscribeToTable } from '../lib/eventStore';

  let datapaths: Record<string, unknown>[] = $state([]);
  let datapathsLoading = $state(true);
  let selectedDatapath = $state('');
  let timeRange = $state<5 | 15 | 30 | 0>(30);
  let timeRangeValue = $state('30');
  let autoRefresh = $state(false);
  let diffData = $state<FlowDiffResponse | null>(null);
  let loading = $state(false);
  let error = $state('');
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;

  const timeRangeOptions = [
    { value: '5', label: '5 min' },
    { value: '15', label: '15 min' },
    { value: '30', label: '30 min' },
    { value: '0', label: 'All' },
  ];

  function onTimeRangeChange(v: string) {
    timeRange = Number(v) as 5 | 15 | 30 | 0;
  }

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

<PageHeader
  eyebrow="Debug"
  title="Flow Diff"
  description="Track LogicalFlow changes over time — inserts, updates, and deletes"
/>

<DataState loading={datapathsLoading} {error}>
  <!-- Controls -->
  <div class="mb-4 flex flex-wrap items-center gap-3">
    <select
      bind:value={selectedDatapath}
      class="select select-bordered select-sm w-64 bg-base-200/60 font-mono"
    >
      <option value="">All datapaths</option>
      {#each datapathOptions as dp (dp.uuid)}
        <option value={dp.uuid}>[{dp.type}] {dp.name}</option>
      {/each}
    </select>

    <SegmentedControl
      options={timeRangeOptions}
      bind:value={timeRangeValue}
      onchange={onTimeRangeChange}
      size="xs"
    />

    <label class="label cursor-pointer gap-2">
      <span
        class="font-mono text-2xs uppercase tracking-wider text-base-content/60"
        >Auto-refresh</span
      >
      <input
        type="checkbox"
        bind:checked={autoRefresh}
        class="toggle toggle-primary toggle-xs"
      />
    </label>

    <button class="btn btn-ghost btn-xs border-base-300" onclick={loadDiff}
      >Refresh</button
    >
  </div>

  {#if loading && !diffData}
    <LoadingSpinner />
  {:else if diffData}
    <!-- Stats bar -->
    <StatTiles
      class="mb-4 w-full"
      tiles={[
        { label: 'Total Changes', value: diffData.count },
        { label: 'Inserts', value: inserts, variant: 'success' },
        { label: 'Updates', value: updates, variant: 'warning' },
        { label: 'Deletes', value: deletes, variant: 'error' },
      ]}
    />

    <!-- Timeline -->
    {#if diffData.changes.length === 0}
      <div class="py-8 text-center">
        <span class="font-mono text-sm text-base-content/40"
          ><span class="text-base-content/30">//</span> no flow changes in the selected
          time range</span
        >
      </div>
    {:else}
      <div class="flex flex-col gap-2">
        {#each diffData.changes as change (change.uuid + change.timestamp)}
          <div
            class="rounded border border-l-4 border-base-300 bg-base-100 px-4 py-3 {changeColor(
              change.type,
            )}"
          >
            <div class="flex items-center gap-2">
              <Badge
                text={change.type}
                variant={actionVariant(change.type)}
                glyph={actionGlyph(change.type)}
              />
              <span class="font-mono text-xs text-base-content/50"
                >{change.uuid.slice(0, 12)}</span
              >
              <span class="ml-auto font-mono text-2xs text-base-content/40"
                >{formatTime(change.timestamp)} ({formatTimeDelta(
                  change.timestamp,
                )})</span
              >
            </div>

            {#if change.type === 'update' && change.old_row && change.new_row}
              <div class="mt-2 grid grid-cols-2 gap-2 font-mono text-xs">
                <div>
                  <div
                    class="mb-0.5 font-semibold uppercase tracking-wider text-base-content/50"
                  >
                    Before
                  </div>
                  {#if change.old_row.match}
                    <div>
                      <span class="text-base-content/50">match:</span>
                      <span>{change.old_row.match}</span>
                    </div>
                  {/if}
                  {#if change.old_row.actions}
                    <div>
                      <span class="text-base-content/50">actions:</span>
                      <span>{change.old_row.actions}</span>
                    </div>
                  {/if}
                </div>
                <div>
                  <div
                    class="mb-0.5 font-semibold uppercase tracking-wider text-base-content/50"
                  >
                    After
                  </div>
                  {#if change.new_row.match}
                    <div>
                      <span class="text-base-content/50">match:</span>
                      <span>{change.new_row.match}</span>
                    </div>
                  {/if}
                  {#if change.new_row.actions}
                    <div>
                      <span class="text-base-content/50">actions:</span>
                      <span>{change.new_row.actions}</span>
                    </div>
                  {/if}
                </div>
              </div>
            {:else if change.new_row}
              <div class="mt-1 font-mono text-xs">
                {#if change.new_row.match}
                  <span class="text-base-content/50">match:</span>
                  <span>{change.new_row.match}</span>
                {/if}
                {#if change.new_row.actions}
                  <span class="ml-2 text-base-content/50">actions:</span>
                  <span>{change.new_row.actions}</span>
                {/if}
              </div>
            {:else if change.old_row}
              <div class="mt-1 font-mono text-xs line-through opacity-60">
                {#if change.old_row.match}
                  <span class="text-base-content/50">match:</span>
                  <span>{change.old_row.match}</span>
                {/if}
                {#if change.old_row.actions}
                  <span class="ml-2 text-base-content/50">actions:</span>
                  <span>{change.old_row.actions}</span>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</DataState>
