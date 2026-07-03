<script lang="ts">
  import { getEntity } from '../lib/api';
  import { link } from '../lib/router';
  import { findTable, getCorrelatedRoute, ovsdbTableName } from '../lib/tables';
  import { isWritableTable } from '../lib/writableTables';
  import { writeEnabled } from '../lib/capabilitiesStore';
  import { getImpact, type ImpactResult } from '../lib/writeApi';
  import CellRenderer from '../components/table/CellRenderer.svelte';
  import ImpactTree from '../components/write/ImpactTree.svelte';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Card from '../components/ui/Card.svelte';
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

  let impactResult: ImpactResult | null = $state(null);
  let impactLoading = $state(false);
  let impactError = $state('');

  async function loadImpact() {
    if (!ovsdbName) return;
    impactLoading = true;
    impactError = '';
    try {
      impactResult = await getImpact(db, ovsdbName, uuid);
    } catch (e) {
      impactError = e instanceof Error ? e.message : 'Failed to load impact';
      impactResult = null;
    } finally {
      impactLoading = false;
    }
  }

  function getRefHref(
    column: string,
  ): ((uuid: string) => string | null) | undefined {
    const ref = tableDef?.references?.[column];
    if (!ref) return undefined;
    return (u: string) => `/${ref.db}/${ref.table}/${u}`;
  }
</script>

<PageHeader
  mono
  title={uuid}
  breadcrumbs={[{ label: tableDef?.label ?? table, href: `/${db}/${table}` }]}
>
  {#snippet actions()}
    {#if correlatedHref}
      <a href={link(correlatedHref)} class="btn btn-outline btn-primary btn-sm">
        Correlated View
      </a>
    {/if}
    {#if $writeEnabled && canWrite}
      <a
        href={link(`/write?action=update&table=${ovsdbName}&uuid=${uuid}`)}
        class="btn btn-outline btn-sm btn-warning"
      >
        Edit
      </a>
      <a
        href={link(`/write?action=delete&table=${ovsdbName}&uuid=${uuid}`)}
        class="btn btn-outline btn-error btn-sm"
      >
        Delete
      </a>
      <button
        class="btn btn-outline btn-info btn-sm"
        disabled={impactLoading}
        onclick={loadImpact}
      >
        {#if impactLoading}
          <span class="loading loading-xs loading-spinner"></span>
        {/if}
        Impact Analysis
      </button>
    {/if}
  {/snippet}
</PageHeader>

{#if impactError}
  <ErrorAlert message={impactError} />
{/if}

{#if impactResult}
  <Card
    title="Impact Analysis"
    subtitle={impactResult.summary.total_affected > 0
      ? `${impactResult.summary.total_affected} dependent object${impactResult.summary.total_affected !== 1 ? 's' : ''}`
      : 'no dependencies'}
    class="mb-4"
  >
    {#if impactResult.summary.total_affected > 0}
      <ImpactTree node={impactResult.root} />
    {:else}
      <span class="font-mono text-sm text-base-content/40">
        <span class="text-base-content/30">//</span> no dependencies
      </span>
    {/if}
  </Card>
{/if}

<DataState {loading} {error} empty={!entity} emptyMessage="entity not found">
  {#if entity}
    <Card title="Properties" padding="none" class="mb-4">
      <table class="table table-sm font-mono">
        <tbody>
          {#each fields as [key, value] (key)}
            <tr class="border-base-300/60">
              <td
                class="w-48 align-top text-xs font-medium whitespace-nowrap text-base-content/55"
                >{key}</td
              >
              <td class="align-top">
                <CellRenderer {value} refHref={getRefHref(key)} />
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </Card>

    <JsonView data={entity} label="Raw JSON" />
  {/if}
</DataState>
