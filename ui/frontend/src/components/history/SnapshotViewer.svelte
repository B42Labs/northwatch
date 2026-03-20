<script lang="ts">
  import {
    getSnapshotRows,
    type SnapshotMeta,
    type SnapshotRow,
  } from '../../lib/api';
  import LoadingSpinner from '../ui/LoadingSpinner.svelte';
  import ErrorAlert from '../ui/ErrorAlert.svelte';

  interface Props {
    snapshot: SnapshotMeta;
    onClose: () => void;
  }

  let { snapshot, onClose }: Props = $props();

  let rows: SnapshotRow[] = $state([]);
  let loading = $state(true);
  let error = $state('');
  let filterDb = $state('');
  let filterTable = $state('');
  let expandedRow: string | null = $state(null);

  let tables = $derived(Object.keys(snapshot.row_counts).sort());
  let databases = $derived([...new Set(tables.map((t) => t.split('.')[0]))]);

  let filteredTables = $derived(
    filterDb ? tables.filter((t) => t.startsWith(filterDb + '.')) : tables,
  );

  async function loadRows() {
    loading = true;
    error = '';
    try {
      rows = await getSnapshotRows(snapshot.id, {
        database: filterDb || undefined,
        table: filterTable || undefined,
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load rows';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void filterDb;
    void filterTable;
    loadRows();
  });
</script>

<div class="rounded-lg border border-base-300 bg-base-100 p-4">
  <div class="mb-3 flex items-center justify-between">
    <div>
      <h3 class="text-lg font-semibold">
        Snapshot #{snapshot.id}
      </h3>
      <p class="text-sm text-base-content/60">
        {new Date(snapshot.timestamp).toLocaleString()}
        {#if snapshot.label}— {snapshot.label}{/if}
      </p>
    </div>
    <button class="btn btn-ghost btn-sm" onclick={onClose}>Close</button>
  </div>

  <div class="mb-3 flex gap-2">
    <select
      bind:value={filterDb}
      class="select select-bordered select-sm"
      onchange={() => (filterTable = '')}
    >
      <option value="">All databases</option>
      {#each databases as db}
        <option value={db}>{db}</option>
      {/each}
    </select>

    <select bind:value={filterTable} class="select select-bordered select-sm">
      <option value="">All tables</option>
      {#each filteredTables as t}
        <option value={t.split('.')[1]}>{t}</option>
      {/each}
    </select>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if loading}
    <LoadingSpinner />
  {:else}
    <div class="mb-2 text-sm text-base-content/60">{rows.length} rows</div>
    <div class="max-h-96 overflow-auto">
      <table class="table table-xs">
        <thead>
          <tr>
            <th>Database</th>
            <th>Table</th>
            <th>UUID</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.database + row.table + row.uuid)}
            {@const rowKey = row.database + '.' + row.table + '.' + row.uuid}
            <tr
              class="hover cursor-pointer"
              onclick={() =>
                (expandedRow = expandedRow === rowKey ? null : rowKey)}
            >
              <td>{row.database}</td>
              <td>{row.table}</td>
              <td class="font-mono text-xs"
                >{row.uuid ? row.uuid.slice(0, 12) : '—'}</td
              >
            </tr>
            {#if expandedRow === rowKey}
              <tr>
                <td colspan="3">
                  <pre
                    class="max-h-48 overflow-auto rounded bg-base-200 p-2 text-xs">{JSON.stringify(
                      row.data,
                      null,
                      2,
                    )}</pre>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
