<script lang="ts">
  import { onMount } from 'svelte';
  import { getTrace, listPortBindings, type TraceResponse } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { SvelteSet } from 'svelte/reactivity';

  let portBindings: Record<string, unknown>[] = $state([]);
  let portsLoading = $state(true);
  let selectedPort = $state('');
  let dstIp = $state('');
  let protocol = $state('');
  let traceData: TraceResponse | null = $state(null);
  let tracing = $state(false);
  let error = $state('');
  let portFilter = $state('');
  let expandedFlows = new SvelteSet<string>();

  interface PBOption {
    uuid: string;
    logicalPort: string;
    type: string;
    chassis: string;
  }

  let pbOptions = $derived<PBOption[]>(
    portBindings
      .map((pb) => ({
        uuid: pb._uuid as string,
        logicalPort: pb.logical_port as string,
        type: (pb.type as string) || 'VIF',
        chassis: pb.chassis ? (pb.chassis as string).slice(0, 8) : 'unbound',
      }))
      .sort((a, b) => a.logicalPort.localeCompare(b.logicalPort)),
  );

  let filteredPBOptions = $derived(
    portFilter
      ? pbOptions.filter(
          (p) =>
            p.logicalPort.toLowerCase().includes(portFilter.toLowerCase()) ||
            p.uuid.toLowerCase().includes(portFilter.toLowerCase()),
        )
      : pbOptions,
  );

  function hintColor(hint: string, selected: boolean): string {
    if (selected && hint === 'likely') return 'border-l-success bg-success/5';
    if (selected && hint === 'possible') return 'border-l-warning bg-warning/5';
    if (hint === 'likely') return 'border-l-success/50';
    if (hint === 'possible') return 'border-l-warning/50';
    if (hint === 'default') return 'border-l-base-300 opacity-60';
    return 'border-l-base-300 opacity-40';
  }

  function hintBadge(hint: string): { label: string; cls: string } | null {
    switch (hint) {
      case 'likely':
        return { label: 'likely', cls: 'badge-success' };
      case 'possible':
        return { label: 'possible', cls: 'badge-warning' };
      case 'default':
        return { label: 'default', cls: 'badge-ghost' };
      default:
        return null;
    }
  }

  async function loadPorts() {
    try {
      portBindings = await listPortBindings();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load port bindings';
    } finally {
      portsLoading = false;
    }
  }

  async function runTrace() {
    if (!selectedPort) return;
    tracing = true;
    error = '';
    traceData = null;
    try {
      traceData = await getTrace(selectedPort, {
        dstIp: dstIp || undefined,
        protocol: protocol || undefined,
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Trace failed';
    } finally {
      tracing = false;
    }
  }

  function toggleFlow(uuid: string) {
    if (expandedFlows.has(uuid)) {
      expandedFlows.delete(uuid);
    } else {
      expandedFlows.add(uuid);
    }
  }

  onMount(() => {
    loadPorts();
  });
</script>

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Packet Trace</h1>
    <p class="text-sm text-base-content/60">
      Simplified ovn-trace: see which logical flows a packet would traverse
      through the pipeline
    </p>
  </div>

  {#if error && !traceData}
    <ErrorAlert message={error} />
  {/if}

  {#if portsLoading}
    <LoadingSpinner />
  {:else}
    <!-- Trace parameters -->
    <div class="mb-4 flex flex-wrap items-end gap-3">
      <div class="w-80">
        <label class="label" for="trace-port">
          <span class="label-text text-xs font-semibold"
            >Source Port (SB Port Binding)</span
          >
        </label>
        <input
          type="text"
          bind:value={portFilter}
          placeholder="Filter ports..."
          class="input input-sm input-bordered mb-1 w-full"
        />
        <select
          id="trace-port"
          bind:value={selectedPort}
          class="select select-bordered select-sm w-full"
        >
          <option value="">Select port...</option>
          {#each filteredPBOptions as p (p.uuid)}
            <option value={p.uuid}
              >{p.logicalPort} ({p.type}, {p.chassis})</option
            >
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="dst-ip">
          <span class="label-text text-xs font-semibold">Destination IP</span>
        </label>
        <input
          id="dst-ip"
          type="text"
          bind:value={dstIp}
          placeholder="e.g. 10.0.0.1"
          class="input input-sm input-bordered w-40"
        />
      </div>
      <div>
        <label class="label" for="protocol">
          <span class="label-text text-xs font-semibold">Protocol</span>
        </label>
        <select
          id="protocol"
          bind:value={protocol}
          class="select select-bordered select-sm"
        >
          <option value="">Any</option>
          <option value="tcp">TCP</option>
          <option value="udp">UDP</option>
          <option value="icmp">ICMP</option>
        </select>
      </div>
      <button
        class="btn btn-primary btn-sm"
        disabled={!selectedPort || tracing}
        onclick={runTrace}
      >
        {#if tracing}
          <span class="loading loading-spinner loading-xs"></span>
        {/if}
        Trace
      </button>
    </div>

    {#if traceData}
      <!-- Trace info -->
      <div
        class="mb-4 rounded-lg border border-base-300 bg-base-100 p-3 text-sm"
      >
        <div class="flex flex-wrap gap-4">
          <span
            >Port: <span class="font-mono font-semibold"
              >{traceData.port_name}</span
            ></span
          >
          <span
            >Datapath: <span class="font-semibold"
              >{traceData.datapath_name ||
                traceData.datapath_uuid.slice(0, 8)}</span
            ></span
          >
          {#if traceData.dst_ip}
            <span
              >Dst IP: <span class="font-mono">{traceData.dst_ip}</span></span
            >
          {/if}
          {#if traceData.protocol}
            <span
              >Protocol: <span class="font-mono">{traceData.protocol}</span
              ></span
            >
          {/if}
        </div>
      </div>

      <!-- Pipeline stepper -->
      <div class="flex flex-col gap-3">
        {#each traceData.stages as stage, i}
          <div class="flex items-start gap-3">
            <!-- Stage marker -->
            <div class="flex flex-col items-center">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold {stage.pipeline ===
                'ingress'
                  ? 'bg-info/20 text-info'
                  : 'bg-warning/20 text-warning'}"
              >
                {stage.table_id}
              </div>
              {#if i < traceData.stages.length - 1}
                <div class="h-full w-0.5 bg-base-300"></div>
              {/if}
            </div>

            <!-- Stage content -->
            <div class="flex-1 rounded-lg border border-base-300 bg-base-100">
              <div
                class="flex items-center gap-2 border-b border-base-300 px-3 py-2"
              >
                <span
                  class="badge badge-xs {stage.pipeline === 'ingress'
                    ? 'badge-info'
                    : 'badge-warning'}">{stage.pipeline}</span
                >
                <span class="text-sm font-semibold">
                  Table {stage.table_id}
                  {#if stage.table_name}
                    <span class="ml-1 font-normal text-base-content/60"
                      >{stage.table_name}</span
                    >
                  {/if}
                </span>
                <span class="text-xs text-base-content/40"
                  >{stage.flows.length} flows</span
                >
              </div>

              <div class="max-h-[300px] overflow-y-auto">
                {#each stage.flows as flow}
                  <button
                    type="button"
                    class="block w-full cursor-pointer border-b border-l-4 border-base-300 px-3 py-1.5 text-left text-xs last:border-0 hover:bg-base-200 {hintColor(
                      flow.hint,
                      flow.selected,
                    )}"
                    onclick={() => toggleFlow(flow.uuid)}
                  >
                    <div class="flex items-center gap-2">
                      {#if flow.selected}
                        <span class="text-success">&#10003;</span>
                      {/if}
                      <span class="badge badge-ghost badge-sm font-mono"
                        >{flow.priority}</span
                      >
                      {#if hintBadge(flow.hint)}
                        <span class="badge badge-xs {hintBadge(flow.hint)?.cls}"
                          >{hintBadge(flow.hint)?.label}</span
                        >
                      {/if}
                      <span class="truncate font-mono text-base-content/80"
                        >{flow.match || '1 (any)'}</span
                      >
                    </div>

                    {#if expandedFlows.has(flow.uuid)}
                      <div
                        class="mt-2 space-y-1 rounded border border-base-300 bg-base-200 p-2"
                      >
                        <div>
                          <span class="font-semibold text-base-content/50"
                            >Match:</span
                          >
                          <span class="font-mono"
                            >{flow.match || '1 (any)'}</span
                          >
                        </div>
                        <div>
                          <span class="font-semibold text-base-content/50"
                            >Actions:</span
                          >
                          <span class="font-mono">{flow.actions}</span>
                        </div>
                        <div class="text-base-content/40">
                          UUID: {flow.uuid}
                        </div>
                      </div>
                    {/if}
                  </button>
                {/each}
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
