<script lang="ts">
  import { onMount } from 'svelte';
  import { listAuditEntries, type AuditEntry } from '../lib/writeApi';
  import PageContainer from '../components/ui/PageContainer.svelte';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Toolbar from '../components/ui/Toolbar.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import JsonView from '../components/ui/JsonView.svelte';

  let { entryId }: { entryId?: string } = $props();

  let entries: AuditEntry[] = $state([]);
  let loading = $state(true);
  let error = $state('');
  let limit = $state(50);
  let resultFilter = $state<'all' | 'success' | 'error'>('all');
  let expandedId: number | null = $state(null);

  onMount(() => {
    load();
  });

  async function load() {
    loading = true;
    error = '';
    try {
      entries = await listAuditEntries(limit);
      // Auto-expand entry from URL param
      if (entryId) {
        expandedId = parseInt(entryId, 10);
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load audit log';
    } finally {
      loading = false;
    }
  }

  let filtered = $derived.by(() => {
    if (resultFilter === 'all') return entries;
    return entries.filter((e) => e.result === resultFilter);
  });

  function toggleExpand(id: number) {
    expandedId = expandedId === id ? null : id;
  }

  function formatTime(ts: string): string {
    const d = new Date(ts);
    return d.toLocaleString();
  }
</script>

<PageContainer width="prose">
  <PageHeader
    eyebrow="Write"
    title="Write Audit Log"
    description="History of all applied write operations."
  />

  <!-- Controls -->
  <Toolbar>
    <SegmentedControl
      options={[
        { value: 'all', label: 'All' },
        { value: 'success', label: 'Success' },
        { value: 'error', label: 'Error' },
      ]}
      value={resultFilter}
      onchange={(v) => (resultFilter = v as 'all' | 'success' | 'error')}
      size="xs"
    />

    <select
      class="select select-bordered select-xs bg-base-200/60 font-mono"
      bind:value={limit}
      onchange={load}
    >
      <option value={25}>25 entries</option>
      <option value={50}>50 entries</option>
      <option value={100}>100 entries</option>
      <option value={250}>250 entries</option>
    </select>

    <button class="btn btn-ghost btn-xs border-base-300 font-mono" onclick={load}
      >&#x21bb; Refresh</button
    >
  </Toolbar>

  <DataState
    {loading}
    {error}
    empty={filtered.length === 0}
    emptyMessage="no audit entries found"
  >
    <div class="flex flex-col gap-2">
      {#each filtered as entry (entry.id)}
        <div class="rounded border border-base-300 bg-base-100">
          <div
            class="cursor-pointer p-3"
            onclick={() => toggleExpand(entry.id)}
            role="button"
            tabindex="0"
            onkeydown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') toggleExpand(entry.id);
            }}
          >
            <div class="flex flex-wrap items-center gap-2">
              <Badge
                text={entry.result}
                variant={entry.result === 'success' ? 'success' : 'error'}
              />
              <span class="font-mono text-xs text-base-content/60">
                {formatTime(entry.timestamp)}
              </span>
              <span class="font-mono text-xs text-base-content/50">
                Plan: {entry.plan_id.slice(0, 12)}
              </span>
              {#if entry.actor}
                <span class="font-mono text-xs">by {entry.actor}</span>
              {/if}
              <span class="font-mono text-xs text-base-content/50">
                {entry.operations.length} op(s)
              </span>
              {#if entry.reason}
                <span class="font-mono text-xs italic text-base-content/40">
                  {entry.reason}
                </span>
              {/if}
              <span class="ml-auto font-mono text-xs text-base-content/30">
                {expandedId === entry.id ? '-' : '+'}
              </span>
            </div>

            {#if entry.error}
              <div class="mt-1 font-mono text-xs text-error">{entry.error}</div>
            {/if}
          </div>

          {#if expandedId === entry.id}
            <div class="border-t border-base-300 p-3">
              <div class="mb-2 grid grid-cols-2 gap-2 font-mono text-xs">
                <div>
                  <span class="font-semibold">Audit ID:</span>
                  {entry.id}
                </div>
                <div>
                  <span class="font-semibold">Snapshot ID:</span>
                  {entry.snapshot_id}
                </div>
                <div>
                  <span class="font-semibold">Actor:</span>
                  {entry.actor || '-'}
                </div>
                <div>
                  <span class="font-semibold">Reason:</span>
                  {entry.reason || '-'}
                </div>
              </div>
              <JsonView data={entry.operations} label="Operations" />
            </div>
          {/if}
        </div>
      {/each}
    </div>
  </DataState>
</PageContainer>
