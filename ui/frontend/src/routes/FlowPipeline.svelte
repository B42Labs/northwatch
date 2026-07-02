<script lang="ts">
  import {
    getFlows,
    listDatapathBindings,
    type FlowPipelineResponse,
    type FlowTableGroup,
  } from '../lib/api';
  import FlowTable from '../components/flows/FlowTable.svelte';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import type { Variant } from '../lib/status';
  import { subscribeToTable } from '../lib/eventStore';

  interface DatapathOption {
    uuid: string;
    name: string;
    shortName: string;
    type: 'switch' | 'router' | 'unknown';
  }

  let datapaths: Record<string, unknown>[] = $state([]);
  let selectedDatapath = $state('');
  let datapathsLoading = $state(true);
  let flowData = $state<FlowPipelineResponse | null>(null);
  let flowsLoading = $state(false);
  let error = $state('');
  let searchQuery = $state('');
  let dpFilter = $state('');
  let pipelineFilter = $state<'all' | 'ingress' | 'egress'>('all');
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  let datapathOptions = $derived<DatapathOption[]>(
    datapaths
      .map((dp) => {
        const uuid = dp._uuid as string;
        const extIds = (dp.external_ids ?? {}) as Record<string, string>;
        const name = extIds['name'] || uuid.slice(0, 8);
        const type: 'switch' | 'router' | 'unknown' = extIds['logical-switch']
          ? 'switch'
          : extIds['logical-router']
            ? 'router'
            : 'unknown';
        const m = name.match(/^neutron-([a-f0-9]{8})/);
        const shortName = m ? m[1] : name;
        return { uuid, name, shortName, type };
      })
      .sort(
        (a, b) => a.type.localeCompare(b.type) || a.name.localeCompare(b.name),
      ),
  );

  let switchCount = $derived(
    datapathOptions.filter((o) => o.type === 'switch').length,
  );
  let routerCount = $derived(
    datapathOptions.filter((o) => o.type === 'router').length,
  );

  let filteredDpOptions = $derived(
    datapathOptions.filter((o) => {
      if (!dpFilter) return true;
      const q = dpFilter.toLowerCase();
      return (
        o.name.toLowerCase().includes(q) ||
        o.uuid.toLowerCase().includes(q) ||
        o.type.includes(q)
      );
    }),
  );

  // Flow stats
  let ingressFlows = $derived(flowData?.ingress ?? []);
  let egressFlows = $derived(flowData?.egress ?? []);
  let totalIngress = $derived(
    ingressFlows.reduce((s, t) => s + t.flows.length, 0),
  );
  let totalEgress = $derived(
    egressFlows.reduce((s, t) => s + t.flows.length, 0),
  );

  let visibleIngress = $derived<FlowTableGroup[]>(
    pipelineFilter === 'egress' ? [] : ingressFlows,
  );
  let visibleEgress = $derived<FlowTableGroup[]>(
    pipelineFilter === 'ingress' ? [] : egressFlows,
  );

  const pipelineOptions = [
    { value: 'all', label: 'All' },
    { value: 'ingress', label: 'Ingress' },
    { value: 'egress', label: 'Egress' },
  ];

  function datapathVariant(type: DatapathOption['type']): Variant {
    return type === 'switch' ? 'info' : type === 'router' ? 'success' : 'ghost';
  }

  function selectDatapath(opt: DatapathOption) {
    selectedDatapath = opt.uuid;
    searchQuery = '';
    pipelineFilter = 'all';
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

  async function loadFlows(uuid: string) {
    if (!uuid) {
      flowData = null;
      return;
    }
    flowsLoading = true;
    error = '';
    try {
      flowData = await getFlows(uuid);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load flows';
      flowData = null;
    } finally {
      flowsLoading = false;
    }
  }

  $effect(() => {
    loadDatapaths();
  });
  $effect(() => {
    if (selectedDatapath) loadFlows(selectedDatapath);
  });

  $effect(() => {
    if (!selectedDatapath) return;
    const unsubscribe = subscribeToTable('sb', 'Logical_Flow', () => {
      if (refetchTimer) clearTimeout(refetchTimer);
      refetchTimer = setTimeout(() => {
        if (!flowsLoading && selectedDatapath) loadFlows(selectedDatapath);
      }, 500);
    });
    return () => {
      unsubscribe();
      if (refetchTimer) clearTimeout(refetchTimer);
    };
  });
</script>

<PageHeader
  eyebrow="Inspect"
  title="Flow Pipeline"
  description="Logical flow tables grouped by pipeline stage — select a datapath to inspect its flows."
>
  {#snippet actions()}
    <StatTiles
      tiles={[
        { label: 'Switches', value: switchCount, variant: 'info' },
        { label: 'Routers', value: routerCount, variant: 'success' },
      ]}
    />
  {/snippet}
</PageHeader>

<DataState loading={datapathsLoading} {error}>
  <!-- Datapath filter -->
  <div class="mb-3 flex flex-wrap items-center gap-2">
    <FilterInput
      bind:value={dpFilter}
      placeholder="filter datapaths…"
      width="w-72"
    />
  </div>

  <div
    class="mb-6 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
  >
    {#each filteredDpOptions as opt (opt.uuid)}
      <button
        class="cursor-pointer rounded border p-3 text-left transition-colors
          {opt.uuid === selectedDatapath
          ? 'border-primary bg-primary/10 ring-1 ring-primary/30'
          : 'border-base-300 bg-base-100 hover:border-base-content/20 hover:bg-base-300/40'}"
        onclick={() => selectDatapath(opt)}
      >
        <div class="flex items-center gap-2">
          <Badge text={opt.type} variant={datapathVariant(opt.type)} />
          <span class="font-mono text-sm font-semibold">{opt.shortName}</span>
        </div>
        <div class="mt-0.5 font-mono text-2xs text-base-content/40">
          {opt.uuid.slice(0, 16)}...
        </div>
      </button>
    {/each}
    {#if filteredDpOptions.length === 0}
      <div
        class="col-span-full py-4 text-center font-mono text-sm text-base-content/40"
      >
        <span class="text-base-content/30">//</span> no datapaths match "{dpFilter}"
      </div>
    {/if}
  </div>

  <DataState loading={flowsLoading}>
    {#if flowData}
      <!-- Flow controls -->
      <div class="mb-4 flex flex-wrap items-center gap-2">
        <FilterInput
          bind:value={searchQuery}
          placeholder="filter match, actions…"
          width="w-72"
        />
        {#if searchQuery}
          <button
            class="btn btn-ghost btn-xs border-base-300 font-mono normal-case"
            onclick={() => (searchQuery = '')}>Clear</button
          >
        {/if}
        <SegmentedControl
          options={pipelineOptions}
          value={pipelineFilter}
          onchange={(v) => (pipelineFilter = v as 'all' | 'ingress' | 'egress')}
          size="xs"
        />

        <StatTiles
          class="ml-auto"
          tiles={[
            {
              label: 'Ingress',
              value: totalIngress,
              variant: 'info',
              hint: `${ingressFlows.length} tables`,
            },
            {
              label: 'Egress',
              value: totalEgress,
              variant: 'warning',
              hint: `${egressFlows.length} tables`,
            },
            { label: 'Total', value: totalIngress + totalEgress },
          ]}
        />
      </div>

      <!-- Pipeline tables -->
      <div class="flex flex-col gap-6">
        {#if visibleIngress.length > 0}
          <div>
            <h3
              class="mb-2 font-mono text-xs font-semibold uppercase tracking-wider text-info"
            >
              Ingress Pipeline
            </h3>
            <div class="flex flex-col gap-3">
              {#each visibleIngress as tbl (tbl.table_id)}
                <FlowTable
                  tableId={tbl.table_id}
                  tableName={tbl.table_name}
                  flows={tbl.flows}
                  pipeline="ingress"
                  {searchQuery}
                />
              {/each}
            </div>
          </div>
        {/if}

        {#if visibleEgress.length > 0}
          <div>
            <h3
              class="mb-2 font-mono text-xs font-semibold uppercase tracking-wider text-warning"
            >
              Egress Pipeline
            </h3>
            <div class="flex flex-col gap-3">
              {#each visibleEgress as tbl (tbl.table_id)}
                <FlowTable
                  tableId={tbl.table_id}
                  tableName={tbl.table_name}
                  flows={tbl.flows}
                  pipeline="egress"
                  {searchQuery}
                />
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {:else if !selectedDatapath}
      <div class="py-4 text-center font-mono text-sm text-base-content/40">
        <span class="text-base-content/30">//</span> select a datapath above to view
        its flow pipeline
      </div>
    {/if}
  </DataState>
</DataState>
