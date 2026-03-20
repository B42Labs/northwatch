<script lang="ts">
  import {
    getFlows,
    listDatapathBindings,
    type FlowPipelineResponse,
    type FlowTableGroup,
  } from '../lib/api';
  import FlowTable from '../components/flows/FlowTable.svelte';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
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
  let flowData: FlowPipelineResponse | null = $state(null);
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

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Flow Pipeline</h1>
    <p class="text-sm text-base-content/60">
      Logical flow tables grouped by pipeline stage — select a datapath to
      inspect its flows
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if datapathsLoading}
    <LoadingSpinner />
  {:else}
    <!-- Datapath selector cards -->
    <div class="mb-4 flex flex-wrap items-center gap-4">
      <div>
        <input
          type="text"
          bind:value={dpFilter}
          placeholder="Filter datapaths..."
          class="input input-sm input-bordered w-72"
        />
      </div>
      <div
        class="stats stats-horizontal ml-auto border border-base-300 bg-base-100 shadow-sm"
      >
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Switches</div>
          <div class="stat-value text-lg">{switchCount}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Routers</div>
          <div class="stat-value text-lg">{routerCount}</div>
        </div>
      </div>
    </div>

    <div
      class="mb-6 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
    >
      {#each filteredDpOptions as opt (opt.uuid)}
        <button
          class="card cursor-pointer border text-left transition-all hover:shadow-md
            {opt.uuid === selectedDatapath
            ? 'border-primary bg-primary/10 shadow-md ring-1 ring-primary/30'
            : 'border-base-300 bg-base-100 hover:border-base-content/20'}"
          onclick={() => selectDatapath(opt)}
        >
          <div class="card-body p-3">
            <div class="flex items-center gap-2">
              <span
                class="badge badge-sm {opt.type === 'switch'
                  ? 'badge-info'
                  : opt.type === 'router'
                    ? 'badge-success'
                    : 'badge-ghost'}"
              >
                {opt.type}
              </span>
              <span class="text-sm font-semibold">{opt.shortName}</span>
            </div>
            <div class="mt-0.5 font-mono text-xs text-base-content/40">
              {opt.uuid.slice(0, 16)}...
            </div>
          </div>
        </button>
      {/each}
      {#if filteredDpOptions.length === 0}
        <div
          class="col-span-full py-4 text-center text-sm text-base-content/40"
        >
          No datapaths match "{dpFilter}"
        </div>
      {/if}
    </div>

    {#if flowsLoading}
      <LoadingSpinner />
    {:else if flowData}
      <!-- Flow controls -->
      <div class="mb-4 flex flex-wrap items-center gap-4">
        <div class="flex items-center gap-3">
          <input
            type="text"
            bind:value={searchQuery}
            placeholder="Search match, actions..."
            class="input input-sm input-bordered w-72"
          />
          {#if searchQuery}
            <button
              class="btn btn-ghost btn-xs"
              onclick={() => (searchQuery = '')}>Clear</button
            >
          {/if}
          <div class="join">
            <button
              class="btn join-item btn-xs {pipelineFilter === 'all'
                ? 'btn-active'
                : ''}"
              onclick={() => (pipelineFilter = 'all')}>All</button
            >
            <button
              class="btn join-item btn-xs {pipelineFilter === 'ingress'
                ? 'btn-active'
                : ''}"
              onclick={() => (pipelineFilter = 'ingress')}>Ingress</button
            >
            <button
              class="btn join-item btn-xs {pipelineFilter === 'egress'
                ? 'btn-active'
                : ''}"
              onclick={() => (pipelineFilter = 'egress')}>Egress</button
            >
          </div>
        </div>

        <div
          class="stats stats-horizontal ml-auto border border-base-300 bg-base-100 shadow-sm"
        >
          <div class="stat px-4 py-2">
            <div class="stat-title text-xs">Ingress</div>
            <div class="stat-value text-lg">{totalIngress}</div>
            <div class="stat-desc text-xs">{ingressFlows.length} tables</div>
          </div>
          <div class="stat px-4 py-2">
            <div class="stat-title text-xs">Egress</div>
            <div class="stat-value text-lg">{totalEgress}</div>
            <div class="stat-desc text-xs">{egressFlows.length} tables</div>
          </div>
          <div class="stat px-4 py-2">
            <div class="stat-title text-xs">Total</div>
            <div class="stat-value text-lg">{totalIngress + totalEgress}</div>
          </div>
        </div>
      </div>

      <!-- Pipeline tables -->
      <div class="flex flex-col gap-6">
        {#if visibleIngress.length > 0}
          <div>
            <h3
              class="mb-2 text-sm font-semibold uppercase tracking-wide text-info"
            >
              Ingress Pipeline
            </h3>
            <div class="flex flex-col gap-3">
              {#each visibleIngress as tbl}
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
              class="mb-2 text-sm font-semibold uppercase tracking-wide text-warning"
            >
              Egress Pipeline
            </h3>
            <div class="flex flex-col gap-3">
              {#each visibleEgress as tbl}
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
      <div class="py-4 text-center text-base-content/50">
        Select a datapath above to view its flow pipeline
      </div>
    {/if}
  {/if}
</div>
