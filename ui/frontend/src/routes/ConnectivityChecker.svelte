<script lang="ts">
  import { onMount } from 'svelte';
  import {
    checkConnectivity,
    listLogicalSwitchPorts,
    type ConnectivityResult,
  } from '../lib/api';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import FormField from '../components/ui/FormField.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import type { Variant } from '../lib/status';

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

  function statusInfo(status: string): { variant: Variant; label: string } {
    switch (status) {
      case 'pass':
        return { variant: 'success', label: 'Pass' };
      case 'fail':
        return { variant: 'error', label: 'Fail' };
      case 'warning':
        return { variant: 'warning', label: 'Warning' };
      default:
        return { variant: 'ghost', label: 'Skipped' };
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

<PageHeader
  eyebrow="Debug"
  title="Connectivity Checker"
  description="Analyze L2/L3 connectivity, ACL rules, and physical realization between two logical ports"
/>

<DataState loading={portsLoading} error={error && !result ? error : ''}>
  <!-- Port selectors -->
  <div class="mb-4 grid grid-cols-1 gap-4 md:grid-cols-2">
    <FormField label="Source Port" forId="src-port">
      <input
        id="src-port"
        type="text"
        bind:value={srcFilter}
        placeholder="Filter ports..."
        class="input input-sm input-bordered mb-1 w-full font-mono"
      />
      <select
        bind:value={srcUuid}
        class="select select-bordered select-sm w-full bg-base-200/60 font-mono"
      >
        <option value="">Select source port...</option>
        {#each filteredSrcPorts as p (p.uuid)}
          <option value={p.uuid}>{p.name}</option>
        {/each}
      </select>
    </FormField>
    <FormField label="Destination Port" forId="dst-port">
      <input
        id="dst-port"
        type="text"
        bind:value={dstFilter}
        placeholder="Filter ports..."
        class="input input-sm input-bordered mb-1 w-full font-mono"
      />
      <select
        bind:value={dstUuid}
        class="select select-bordered select-sm w-full bg-base-200/60 font-mono"
      >
        <option value="">Select destination port...</option>
        {#each filteredDstPorts as p (p.uuid)}
          <option value={p.uuid}>{p.name}</option>
        {/each}
      </select>
    </FormField>
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
        >Overall: {statusInfo(result.overall).label}</span
      >
    </div>

    <!-- Source/destination info -->
    <div class="mb-4 grid grid-cols-1 gap-4 md:grid-cols-2">
      <div class="rounded border border-base-300 bg-base-100 p-3">
        <div
          class="mb-1 font-mono text-2xs font-semibold uppercase tracking-wider text-base-content/55"
        >
          Source
        </div>
        <div class="font-semibold">{result.source.name}</div>
        {#if result.source.switch_name}
          <div class="font-mono text-xs text-base-content/60">
            Switch: {result.source.switch_name}
          </div>
        {/if}
        {#if result.source.addresses?.length}
          <div class="font-mono text-xs text-base-content/60">
            {result.source.addresses.join(', ')}
          </div>
        {/if}
      </div>
      <div class="rounded border border-base-300 bg-base-100 p-3">
        <div
          class="mb-1 font-mono text-2xs font-semibold uppercase tracking-wider text-base-content/55"
        >
          Destination
        </div>
        <div class="font-semibold">{result.destination.name}</div>
        {#if result.destination.switch_name}
          <div class="font-mono text-xs text-base-content/60">
            Switch: {result.destination.switch_name}
          </div>
        {/if}
        {#if result.destination.addresses?.length}
          <div class="font-mono text-xs text-base-content/60">
            {result.destination.addresses.join(', ')}
          </div>
        {/if}
      </div>
    </div>

    <!-- Check results -->
    <div class="flex flex-col gap-2">
      {#each result.checks as check (check.name)}
        <div
          class="flex items-start gap-3 rounded border border-l-4 border-base-300 bg-base-100 px-4 py-2 {categoryColor(
            check.category,
          )}"
        >
          <span class="mt-0.5">
            <Badge
              text={statusInfo(check.status).label}
              variant={statusInfo(check.status).variant}
            />
          </span>
          <div class="flex-1">
            <div class="flex items-center gap-2">
              <span class="font-mono text-xs text-base-content/55"
                >{check.name}</span
              >
              <span class="badge badge-ghost badge-xs">{check.category}</span>
            </div>
            <div class="font-prose text-sm">{check.message}</div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</DataState>
