<script lang="ts">
  import { getEntity } from '../lib/api';
  import { link } from '../lib/router';
  import { findTable, getCorrelatedRoute, ovsdbTableName } from '../lib/tables';
  import { isWritableTable } from '../lib/writableTables';
  import { writeEnabled } from '../lib/capabilitiesStore';
  import CellRenderer from '../components/table/CellRenderer.svelte';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import JsonView from '../components/ui/JsonView.svelte';

  let { db, table, uuid }: { db: string; table: string; uuid: string } =
    $props();

  let entity: Record<string, unknown> | null = $state(null);
  let loading = $state(true);
  let error = $state('');

  let tableDef = $derived(findTable(db, table));

  async function load(
    targetDb: string,
    targetTable: string,
    targetUuid: string,
  ) {
    loading = true;
    error = '';
    try {
      entity = await getEntity(targetDb, targetTable, targetUuid);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load entity';
      entity = null;
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load(db, table, uuid);
  });

  let fields = $derived.by(() => {
    if (!entity) return [];
    return Object.entries(entity)
      .filter(([k]) => k !== '_uuid')
      .sort(([a], [b]) => a.localeCompare(b));
  });

  let correlatedRoute = $derived(getCorrelatedRoute(db, table));
  let correlatedHref = $derived(
    correlatedRoute ? `${correlatedRoute}/${uuid}` : null,
  );
  let ovsdbName = $derived(ovsdbTableName(db, table));
  let canWrite = $derived(!!ovsdbName && isWritableTable(ovsdbName));

  function getRefHref(
    column: string,
  ): ((uuid: string) => string | null) | undefined {
    const ref = tableDef?.references?.[column];
    if (!ref) return undefined;
    return (u: string) => `/${ref.db}/${ref.table}/${u}`;
  }
</script>

<div>
  <div class="mb-1 flex items-center gap-2">
    <a
      href={link(`/${db}/${table}`)}
      class="link-hover link text-sm text-base-content/60"
    >
      {tableDef?.label ?? table}
    </a>
    <span class="text-base-content/40">/</span>
  </div>

  <div class="mb-4 flex flex-wrap items-center gap-3">
    <h1 class="font-mono text-xl font-bold">{uuid}</h1>
    {#if correlatedHref}
      <a href={link(correlatedHref)} class="btn btn-outline btn-primary btn-sm">
        Correlated View
      </a>
    {/if}
    {#if $writeEnabled && canWrite}
      <a
        href={link(`/write?action=update&table=${ovsdbName}&uuid=${uuid}`)}
        class="btn btn-outline btn-warning btn-sm"
      >
        Edit
      </a>
      <a
        href={link(`/write?action=delete&table=${ovsdbName}&uuid=${uuid}`)}
        class="btn btn-outline btn-error btn-sm"
      >
        Delete
      </a>
    {/if}
  </div>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else if entity}
    <div class="card mb-4 bg-base-100 shadow-sm">
      <div class="card-body p-4">
        <table class="table table-sm">
          <tbody>
            {#each fields as [key, value]}
              <tr>
                <td class="w-48 whitespace-nowrap text-sm font-semibold"
                  >{key}</td
                >
                <td>
                  <CellRenderer
                    {value}
                    column={key}
                    refHref={getRefHref(key)}
                  />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <JsonView data={entity} label="Raw JSON" />
  {/if}
</div>
