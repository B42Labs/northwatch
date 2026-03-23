<script lang="ts">
  import { onMount } from 'svelte';
  import {
    checkConnectivity,
    listLogicalSwitchPorts,
    type ConnectivityResult,
  } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';

  let ports: Record<string, unknown>[] = $state([]);
  let portsLoading = $state(true);
  let srcUuid = $state('');
  let dstUuid = $state('');
  let srcFilter = $state('');
  let dstFilter = $state('');
  let result: ConnectivityResult | null = $state(null);
  let checking = $state(false);
  let error = $state('');

  interface PortOption {
    uuid: string;
    name: string;
  }

  let portOptions = $derived<PortOption[]>(
    ports.map((p) => ({
      uuid: p._uuid as string,
      name: (p.name as string) || (p._uuid as string).slice(0, 8),
    })),
  );

  let filteredSrcPorts = $derived(
    srcFilter
      ? portOptions.filter(
          (p) =>
            p.name.toLowerCase().includes(srcFilter.toLowerCase()) ||
            p.uuid.toLowerCase().includes(srcFilter.toLowerCase()),
        )
      : portOptions,
  );

  let filteredDstPorts = $derived(
    dstFilter
      ? portOptions.filter(
          (p) =>
            p.name.toLowerCase().includes(dstFilter.toLowerCase()) ||
            p.uuid.toLowerCase().includes(dstFilter.toLowerCase()),
        )
      : portOptions,
  );

  function statusBadge(status: string): { cls: string; label: string } {
    switch (status) {
      case 'pass':
        return { cls: 'badge-success', label: 'Pass' };
      case 'fail':
        return { cls: 'badge-error', label: 'Fail' };
      case 'warning':
        return { cls: 'badge-warning', label: 'Warning' };
      default:
        return { cls: 'badge-ghost', label: 'Skipped' };
    }
  }

  function categoryColor(cat: string): string {
    switch (cat) {
      case 'resolution':
        return 'border-l-info';
      case 'l2':
        return 'border-l-primary';
      case 'l3':
        return 'border-l-secondary';
      case 'acl':
        return 'border-l-warning';
      case 'physical':
        return 'border-l-accent';
      default:
        return 'border-l-base-300';
    }
  }

  async function loadPorts() {
    try {
      ports = await listLogicalSwitchPorts();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load ports';
    } finally {
      portsLoading = false;
    }
  }

  async function runCheck() {
    if (!srcUuid || !dstUuid) return;
    checking = true;
    error = '';
    result = null;
    try {
      result = await checkConnectivity(srcUuid, dstUuid);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Connectivity check failed';
    } finally {
      checking = false;
    }
  }

  onMount(() => {
    loadPorts();
  });
</script>

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Connectivity Checker</h1>
    <p class="text-sm text-base-content/60">
      Analyze L2/L3 connectivity, ACL rules, and physical realization between
      two logical ports
    </p>
  </div>

  {#if error && !result}
    <ErrorAlert message={error} />
  {/if}

  {#if portsLoading}
    <LoadingSpinner />
  {:else}
    <!-- Port selectors -->
    <div class="mb-4 grid grid-cols-1 gap-4 md:grid-cols-2">
      <div>
        <label class="label" for="src-port">
          <span class="label-text font-semibold">Source Port</span>
        </label>
        <input
          id="src-port"
          type="text"
          bind:value={srcFilter}
          placeholder="Filter ports..."
          class="input input-sm input-bordered mb-1 w-full"
        />
        <select
          bind:value={srcUuid}
          class="select select-bordered select-sm w-full"
        >
          <option value="">Select source port...</option>
          {#each filteredSrcPorts as p (p.uuid)}
            <option value={p.uuid}>{p.name}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="dst-port">
          <span class="label-text font-semibold">Destination Port</span>
        </label>
        <input
          id="dst-port"
          type="text"
          bind:value={dstFilter}
          placeholder="Filter ports..."
          class="input input-sm input-bordered mb-1 w-full"
        />
        <select
          bind:value={dstUuid}
          class="select select-bordered select-sm w-full"
        >
          <option value="">Select destination port...</option>
          {#each filteredDstPorts as p (p.uuid)}
            <option value={p.uuid}>{p.name}</option>
          {/each}
        </select>
      </div>
    </div>

    <button
      class="btn btn-primary btn-sm mb-6"
      disabled={!srcUuid || !dstUuid || checking}
      onclick={runCheck}
    >
      {#if checking}
        <span class="loading loading-spinner loading-xs"></span>
      {/if}
      Check Connectivity
    </button>

    {#if result}
      <!-- Overall status -->
      <div
        class="alert mb-4 {result.overall === 'pass'
          ? 'alert-success'
          : result.overall === 'fail'
            ? 'alert-error'
            : 'alert-warning'}"
      >
        <span class="font-semibold"
          >Overall: {statusBadge(result.overall).label}</span
        >
      </div>

      <!-- Source/destination info -->
      <div class="mb-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        <div class="rounded-lg border border-base-300 bg-base-100 p-3">
          <div
            class="mb-1 text-xs font-semibold uppercase text-base-content/50"
          >
            Source
          </div>
          <div class="font-semibold">{result.source.name}</div>
          {#if result.source.switch_name}
            <div class="text-xs text-base-content/60">
              Switch: {result.source.switch_name}
            </div>
          {/if}
          {#if result.source.addresses?.length}
            <div class="text-xs text-base-content/60">
              {result.source.addresses.join(', ')}
            </div>
          {/if}
        </div>
        <div class="rounded-lg border border-base-300 bg-base-100 p-3">
          <div
            class="mb-1 text-xs font-semibold uppercase text-base-content/50"
          >
            Destination
          </div>
          <div class="font-semibold">{result.destination.name}</div>
          {#if result.destination.switch_name}
            <div class="text-xs text-base-content/60">
              Switch: {result.destination.switch_name}
            </div>
          {/if}
          {#if result.destination.addresses?.length}
            <div class="text-xs text-base-content/60">
              {result.destination.addresses.join(', ')}
            </div>
          {/if}
        </div>
      </div>

      <!-- Check results -->
      <div class="flex flex-col gap-2">
        {#each result.checks as check (check.name)}
          <div
            class="flex items-start gap-3 rounded-lg border-l-4 bg-base-100 px-4 py-2 shadow-sm {categoryColor(
              check.category,
            )}"
          >
            <span class="badge badge-sm mt-0.5 {statusBadge(check.status).cls}"
              >{statusBadge(check.status).label}</span
            >
            <div class="flex-1">
              <div class="flex items-center gap-2">
                <span class="font-mono text-xs text-base-content/50"
                  >{check.name}</span
                >
                <span class="badge badge-ghost badge-xs">{check.category}</span>
              </div>
              <div class="text-sm">{check.message}</div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
