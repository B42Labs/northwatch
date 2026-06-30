<script lang="ts">
  import { getOvsEntity, OVS_TABLES } from '../lib/api';
  import { interfaceSignals, mapSections, type Signal } from '../lib/ovsDetail';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import Card from '../components/ui/Card.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import KeyValueTable from '../components/ui/KeyValueTable.svelte';
  import PropertyCard from '../components/profile/PropertyCard.svelte';
  import JsonView from '../components/ui/JsonView.svelte';

  let {
    chassis,
    table,
    uuid,
  }: { chassis: string; table: string; uuid: string } = $props();

  let entity: Record<string, unknown> | null = $state(null);
  let loading = $state(true);
  let error = $state('');

  let reqId = 0;
  async function load(
    targetChassis: string,
    targetTable: string,
    targetUuid: string,
  ) {
    const myId = ++reqId;
    loading = true;
    error = '';
    try {
      const result = await getOvsEntity(targetChassis, targetTable, targetUuid);
      if (myId !== reqId) return;
      entity = result;
    } catch (e) {
      if (myId !== reqId) return;
      error = e instanceof Error ? e.message : 'Failed to load entity';
      entity = null;
    } finally {
      if (myId === reqId) loading = false;
    }
  }

  $effect(() => {
    load(chassis, table, uuid);
  });

  let tableLabel = $derived(
    OVS_TABLES.find((t) => t.slug === table)?.label ?? table,
  );
  let title = $derived.by(() => {
    const name = entity?.name;
    return typeof name === 'string' && name ? name : uuid;
  });
  // Map-typed fields become their own key/value sections; the Properties card
  // shows the remaining scalars/arrays, so exclude the section keys (plus the
  // identifiers already shown in the header) from it.
  let sections = $derived(entity ? mapSections(entity) : []);
  let signals: Signal[] = $derived(
    entity && table === 'interface' ? interfaceSignals(entity) : [],
  );
  let excluded = $derived(['_uuid', 'name', ...sections.map((s) => s.key)]);
</script>

<PageHeader
  mono
  {title}
  breadcrumbs={[
    { label: 'OVS Visibility', href: '/ovs' },
    { label: tableLabel },
    { label: title },
  ]}
>
  {#snippet meta()}
    <Badge text={chassis} variant="primary" />
    <span class="font-mono text-2xs text-base-content/45">{uuid}</span>
  {/snippet}
</PageHeader>

<DataState {loading} {error} empty={!entity} emptyMessage="row not found">
  {#if entity}
    {#if signals.length > 0}
      <div class="mb-4 flex flex-col gap-1.5">
        <span
          class="font-mono text-2xs uppercase tracking-wider text-base-content/45"
          >Interface signals</span
        >
        <StatTiles tiles={signals} />
      </div>
    {/if}

    <div class="flex flex-col gap-4">
      <PropertyCard title="Properties" data={entity} exclude={excluded} />

      {#each sections as section (section.key)}
        <Card title={section.key} padding="none">
          <KeyValueTable data={section.data} />
        </Card>
      {/each}

      <JsonView data={entity} label="Raw JSON" />
    </div>
  {/if}
</DataState>
