<script lang="ts">
  import { listTable } from '../lib/api';
  import { push, link } from '../lib/router';
  import { findTable, type TableDef, ovsdbTableName } from '../lib/tables';
  import DataTable from '../components/table/DataTable.svelte';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { getCorrelatedListRoute } from '../lib/tables';
  import { subscribeToTable } from '../lib/eventStore';
  import { changedUuids as changedUuidsStore } from '../lib/eventStore';
  import type { WsEvent } from '../lib/websocket';

  let { db, table }: { db: string; table: string } = $props();

  let rows: Record<string, unknown>[] = $state([]);
  let loading = $state(true);
  let error = $state('');
  let tableDef: TableDef | undefined = $state(undefined);
  let currentChangedUuids = $derived($changedUuidsStore);

  // Derive columns from the first 50 rows. OVSDB rows are homogeneous so this
  // captures all columns; the limit avoids scanning very large result sets.
  let allColumns = $derived.by(() => {
    if (rows.length === 0) return [];
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local temp, not reactive state
    const keys = new Set<string>();
    for (const row of rows.slice(0, 50)) {
      for (const key of Object.keys(row)) keys.add(key);
    }
    // Put _uuid first
    const sorted = [...keys].filter((k) => k !== '_uuid').sort();
    return ['_uuid', ...sorted];
  });

  let primaryColumns = $derived(
    tableDef?.primaryColumns ?? allColumns.slice(0, 6),
  );

  async function load(targetDb: string, targetTable: string) {
    loading = true;
    error = '';
    tableDef = findTable(targetDb, targetTable);
    try {
      rows = await listTable(targetDb, targetTable);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data';
      rows = [];
    } finally {
      loading = false;
    }
  }

  let unsubscribeWs: (() => void) | null = null;

  function handleWsEvent(event: WsEvent) {
    if (event.type === 'insert' && event.row) {
      rows = [...rows, event.row];
    } else if (event.type === 'update' && event.row) {
      rows = rows.map((r) => (r._uuid === event.uuid ? event.row! : r));
    } else if (event.type === 'delete') {
      rows = rows.filter((r) => r._uuid !== event.uuid);
    }
  }

  $effect(() => {
    load(db, table);

    // Clean up previous subscription
    unsubscribeWs?.();
    const ovsdbName = ovsdbTableName(db, table);
    if (ovsdbName) {
      unsubscribeWs = subscribeToTable(db, ovsdbName, handleWsEvent);
    }

    return () => {
      unsubscribeWs?.();
      unsubscribeWs = null;
    };
  });

  function handleRowClick(row: Record<string, unknown>) {
    const uuid = row._uuid as string;
    if (uuid) push(`/${db}/${table}/${uuid}`);
  }

  function getRefHref(
    column: string,
  ): ((uuid: string) => string | null) | undefined {
    const ref = tableDef?.references?.[column];
    if (!ref) return undefined;
    return (uuid: string) => `/${ref.db}/${ref.table}/${uuid}`;
  }

  let correlatedRoute = $derived(getCorrelatedListRoute(db, table));
</script>

<div>
  <div class="mb-4 flex items-center gap-3">
    <div>
      <h1 class="text-xl font-bold">{tableDef?.label ?? table}</h1>
      <p class="text-sm text-base-content/60">
        {db === 'nb' ? 'Northbound' : 'Southbound'}
      </p>
    </div>
    {#if correlatedRoute}
      <a
        href={link(correlatedRoute)}
        class="btn btn-outline btn-primary btn-sm"
      >
        Correlated View
      </a>
    {/if}
  </div>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else}
    <DataTable
      {rows}
      columns={primaryColumns}
      {allColumns}
      onRowClick={handleRowClick}
      refHref={getRefHref}
      changedUuids={currentChangedUuids}
    />
  {/if}
</div>
