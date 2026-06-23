<script lang="ts">
  import {
    getSnapshotRows,
    type SnapshotMeta,
    type SnapshotRow,
  } from '../../lib/api';
  import DataState from '../ui/DataState.svelte';

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

<div class="rounded border border-base-300 bg-base-100 p-4">
  <div class="mb-3 flex items-center justify-between">
    <div>
      <h3
        class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
      >
        Snapshot #{snapshot.id}
      </h3>
      <p class="font-mono text-sm text-base-content/60">
        {new Date(snapshot.timestamp).toLocaleString()}
        {#if snapshot.label}— {snapshot.label}{/if}
      </p>
    </div>
    <button class="btn btn-ghost btn-sm border-base-300" onclick={onClose}
      >Close</button
    >
  </div>

  <div class="mb-3 flex gap-2">
    <select
      bind:value={filterDb}
      class="select select-bordered select-sm bg-base-200/60 font-mono"
      onchange={() => (filterTable = '')}
    >
      <option value="">All databases</option>
      {#each databases as db (db)}
        <option value={db}>{db}</option>
      {/each}
    </select>

    <select
      bind:value={filterTable}
      class="select select-bordered select-sm bg-base-200/60 font-mono"
    >
      <option value="">All tables</option>
      {#each filteredTables as t (t)}
        <option value={t.split('.')[1]}>{t}</option>
      {/each}
    </select>
  </div>

  <DataState {loading} {error}>
    <div class="mb-2 font-mono text-sm text-base-content/60">
      {rows.length} rows
    </div>
    <div class="max-h-96 overflow-auto rounded border border-base-300">
      <table class="table table-xs font-mono">
        <thead>
          <tr>
            <th
              class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
              >Database</th
            >
            <th
              class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
              >Table</th
            >
            <th
              class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
              >UUID</th
            >
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.database + row.table + row.uuid)}
            {@const rowKey = row.database + '.' + row.table + '.' + row.uuid}
            <tr
              class="cursor-pointer border-base-300/60 hover:bg-base-300/40"
              onclick={() =>
                (expandedRow = expandedRow === rowKey ? null : rowKey)}
            >
              <td>{row.database}</td>
              <td>{row.table}</td>
              <td class="text-xs"
                >{row.uuid ? row.uuid.slice(0, 12) : '—'}</td
              >
            </tr>
            {#if expandedRow === rowKey}
              <tr>
                <td colspan="3" class="bg-base-200/40">
                  <pre
                    class="max-h-48 overflow-auto rounded bg-base-100 p-2 text-xs">{JSON.stringify(
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
  </DataState>
</div>
