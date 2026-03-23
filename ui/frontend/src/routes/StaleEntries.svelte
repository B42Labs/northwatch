<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';

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

  let filtered = $derived(
    (data?.entries ?? []).filter(
      (e) => typeFilter === 'all' || e.type === typeFilter,
    ),
  );

  function formatAge(seconds?: number): string {
    if (!seconds) return '';
    const hours = Math.floor(seconds / 3600);
    if (hours > 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
    return `${hours}h`;
  }

  async function load() {
    loading = true;
    error = '';
    try {
      data = await get<StaleResult>('/api/v1/debug/stale-entries');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

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
            onclick={() => (typeFilter = val as typeof typeFilter)}
            >{label}</button
          >
        {/each}
      </div>
      <span class="text-sm text-base-content/50">{filtered.length} entries</span
      >
    </div>

    <div class="overflow-x-auto">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Severity</th>
            <th>Type</th>
            <th>Table</th>
            <th>Message</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as entry (entry.uuid)}
            <tr>
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
