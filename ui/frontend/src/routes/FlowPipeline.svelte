<script lang="ts">
  import {
    getFlows,
    listDatapathBindings,
    type FlowPipelineResponse,
  } from '../lib/api';
  import PipelineView from '../components/flows/PipelineView.svelte';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { subscribeToTable } from '../lib/eventStore';

  // Datapath selector state
  let datapaths: Record<string, unknown>[] = $state([]);
  let selectedDatapath = $state('');
  let datapathsLoading = $state(true);

  // Flow data state
  let flowData: FlowPipelineResponse | null = $state(null);
  let flowsLoading = $state(false);
  let error = $state('');
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  // Derive datapath display entries
  let datapathOptions = $derived(
    datapaths
      .map((dp) => {
        const uuid = dp._uuid as string;
        const extIds = (dp.external_ids ?? {}) as Record<string, string>;
        const name =
          extIds['logical-switch'] ||
          extIds['logical-router'] ||
          uuid.slice(0, 8);
        return { uuid, name };
      })
      .sort((a, b) => a.name.localeCompare(b.name)),
  );

  async function loadDatapaths() {
    try {
      datapaths = await listDatapathBindings();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load datapaths';
    } finally {
      datapathsLoading = false;
    }
  }

  async function loadFlows(datapathUuid: string) {
    if (!datapathUuid) {
      flowData = null;
      return;
    }
    flowsLoading = true;
    error = '';
    try {
      flowData = await getFlows(datapathUuid);
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
    if (selectedDatapath) {
      loadFlows(selectedDatapath);
    }
  });

  // Live update: re-fetch flows when Logical_Flow changes
  $effect(() => {
    if (!selectedDatapath) return;

    const unsubscribe = subscribeToTable('sb', 'Logical_Flow', () => {
      if (refetchTimer) clearTimeout(refetchTimer);
      refetchTimer = setTimeout(() => {
        if (!flowsLoading && selectedDatapath) {
          loadFlows(selectedDatapath);
        }
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
      Logical flow tables grouped by pipeline stage
    </p>
  </div>

  <div class="mb-4">
    {#if datapathsLoading}
      <LoadingSpinner />
    {:else}
      <select
        class="select select-bordered select-sm w-full max-w-md"
        bind:value={selectedDatapath}
      >
        <option value="">Select a datapath...</option>
        {#each datapathOptions as dp}
          <option value={dp.uuid}>{dp.name} ({dp.uuid.slice(0, 8)})</option>
        {/each}
      </select>
    {/if}
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if flowsLoading}
    <LoadingSpinner />
  {:else if flowData}
    <div class="mb-2 text-sm text-base-content/60">
      Datapath: <span class="font-semibold"
        >{flowData.datapath_name || flowData.datapath_uuid}</span
      >
    </div>

    <div class="flex flex-col gap-6">
      <PipelineView label="Ingress" tables={flowData.ingress} />
      <PipelineView label="Egress" tables={flowData.egress} />
    </div>
  {:else if selectedDatapath}
    <div class="py-8 text-center text-base-content/50">No flow data</div>
  {:else}
    <div class="py-8 text-center text-base-content/50">
      Select a datapath to view its flow pipeline
    </div>
  {/if}
</div>
