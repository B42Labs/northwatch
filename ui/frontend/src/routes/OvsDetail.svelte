<script lang="ts">
  import {
    getOvsEntity,
    getOvsInterfaceCorrelation,
    listOvsTable,
    OVS_TABLES,
    type OvsInterfaceCorrelation,
  } from '../lib/api';
  import { interfaceSignals, mapSections, type Signal } from '../lib/ovsDetail';
  import { correlationStatus, upVariant } from '../lib/ovsCorrelate';
  import {
    buildLabelIndex,
    ovsRefHref,
    ovsRefLabel,
    referenceTargets,
  } from '../lib/ovsRefs';
  import { link } from '../lib/router';
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
  // UUID → label for this row's resolved reference targets, fetched from the
  // same chassis so reference cells show a name and link through.
  let refIndex = $state<Map<string, string>>(new Map());
  // OVN correlation for an interface row: the Southbound Port_Binding its
  // iface-id realizes. Null for non-interface rows or when correlation fails.
  let correlation = $state<OvsInterfaceCorrelation | null>(null);

  let reqId = 0;
  async function load(
    targetChassis: string,
    targetTable: string,
    targetUuid: string,
  ) {
    const myId = ++reqId;
    loading = true;
    error = '';
    refIndex = new Map();
    correlation = null;
    try {
      const result = await getOvsEntity(targetChassis, targetTable, targetUuid);
      if (myId !== reqId) return;
      entity = result;
      // Interface rows also carry their OVN correlation: the live interface's
      // iface-id resolved to the Southbound Port_Binding it realizes. Best-
      // effort — a failed or absent correlation degrades to no section rather
      // than breaking the entity view.
      if (targetTable === 'interface') {
        const corr = await getOvsInterfaceCorrelation(
          targetChassis,
          targetUuid,
        ).catch(() => null);
        if (myId !== reqId) return;
        correlation = corr;
      }
      // Resolve this row's reference columns by fetching the labelled target
      // tables for the same chassis. Per-target failures degrade to short
      // UUIDs (labels only), so a transient target never breaks the entity.
      const targets = referenceTargets(targetTable);
      if (targets.length > 0) {
        const fetched = await Promise.all(
          targets.map((slug) =>
            listOvsTable(targetChassis, slug).catch(() => []),
          ),
        );
        if (myId !== reqId) return;
        refIndex = buildLabelIndex(
          targets.map((slug, i) => ({ slug, rows: fetched[i] })),
        );
      }
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
  let refHref = $derived(ovsRefHref(chassis, table));
  let refLabel = $derived(ovsRefLabel(refIndex, table));
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
  let corrStatus = $derived(
    correlation ? correlationStatus(correlation) : null,
  );
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
      {#if table === 'interface' && correlation && corrStatus}
        {@const b = correlation.binding}
        <Card title="OVN correlation">
          <div class="flex flex-col gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <Badge text={corrStatus.label} variant={corrStatus.variant} />
              {#if b}
                {#if b.up === undefined}
                  <Badge text="up unknown" variant="neutral" />
                {:else}
                  <Badge
                    text={b.up ? 'up' : 'down'}
                    variant={upVariant(b.up)}
                    glyph={b.up ? '●' : '○'}
                  />
                {/if}
              {/if}
            </div>

            {#if b}
              <dl
                class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 font-mono text-xs"
              >
                <dt class="text-base-content/45">logical_port</dt>
                <dd class="break-all">{b.logical_port}</dd>
                {#if b.datapath}
                  <dt class="text-base-content/45">datapath</dt>
                  <dd class="break-all">{b.datapath}</dd>
                {/if}
                {#if b.chassis}
                  <dt class="text-base-content/45">chassis</dt>
                  <dd class="flex flex-wrap items-center gap-2 break-all">
                    {b.chassis}
                    <Badge
                      text={b.bound_here ? 'this chassis' : 'elsewhere'}
                      variant={b.bound_here ? 'success' : 'warning'}
                    />
                    <a
                      class="text-primary hover:underline"
                      href={link(
                        `/chassis-inventory?chassis=${encodeURIComponent(b.chassis)}`,
                      )}>View SB chassis →</a
                    >
                  </dd>
                {/if}
              </dl>

              {#if correlation.drift && correlation.drift.length > 0}
                <ul class="flex flex-col gap-1">
                  {#each correlation.drift as d (d)}
                    <li class="flex items-start gap-2 text-xs text-error">
                      <span class="text-base-content/30" aria-hidden="true"
                        >//</span
                      >
                      <span>{d}</span>
                    </li>
                  {/each}
                </ul>
              {/if}
            {:else}
              <p class="font-mono text-xs text-base-content/55">
                {#if correlation.iface_id}
                  no Southbound Port_Binding for iface-id <span
                    class="text-base-content/80">{correlation.iface_id}</span
                  >
                {:else}
                  interface has no iface-id — not managed by OVN
                {/if}
              </p>
            {/if}
          </div>
        </Card>
      {/if}

      <PropertyCard
        title="Properties"
        data={entity}
        exclude={excluded}
        {refHref}
        {refLabel}
      />

      {#each sections as section (section.key)}
        <Card title={section.key} padding="none">
          <KeyValueTable data={section.data} />
        </Card>
      {/each}

      <JsonView data={entity} label="Raw JSON" />
    </div>
  {/if}
</DataState>
