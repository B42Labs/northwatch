<script lang="ts">
  import { onMount } from 'svelte';
  import { getPortDiagnostics, type PortDiagnosticsSummary } from '../lib/api';
  import { link } from '../lib/router';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { SvelteSet } from 'svelte/reactivity';

  let data: PortDiagnosticsSummary | null = $state(null);
  let loading = $state(true);
  let error = $state('');
  let searchQuery = $state('');
  let severityFilter = $state<'all' | 'error' | 'warning' | 'healthy'>('all');

  let expandedPorts = new SvelteSet<string>();

  function togglePort(uuid: string) {
    if (expandedPorts.has(uuid)) {
      expandedPorts.delete(uuid);
    } else {
      expandedPorts.add(uuid);
    }
  }

  let filteredPorts = $derived(
    (data?.ports ?? []).filter((p) => {
      if (severityFilter !== 'all' && p.overall !== severityFilter)
        return false;
      if (!searchQuery) return true;
      const q = searchQuery.toLowerCase();
      return (
        p.port_name.toLowerCase().includes(q) ||
        p.port_uuid.toLowerCase().includes(q) ||
        (p.switch_name ?? '').toLowerCase().includes(q) ||
        p.port_type.toLowerCase().includes(q)
      );
    }),
  );

  function severityColor(s: string): string {
    switch (s) {
      case 'error':
        return 'border-error text-error';
      case 'warning':
        return 'border-warning text-warning';
      default:
        return 'border-success text-success';
    }
  }

  function severityBadge(s: string): string {
    switch (s) {
      case 'error':
        return 'badge-error';
      case 'warning':
        return 'badge-warning';
      default:
        return 'badge-success';
    }
  }

  function checkStatusBadge(s: string): string {
    switch (s) {
      case 'error':
        return 'badge-error';
      case 'warning':
        return 'badge-warning';
      default:
        return 'badge-success';
    }
  }

  async function load() {
    loading = true;
    error = '';
    try {
      data = await getPortDiagnostics();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load diagnostics';
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
  });
</script>

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Port Diagnostics</h1>
    <p class="text-sm text-base-content/60">
      Health analysis of all logical ports: binding status, chassis health, type
      verification
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if loading}
    <LoadingSpinner />
  {:else if data}
    <!-- Summary stats -->
    <div class="stats mb-4 w-full border border-base-300 bg-base-100 shadow-sm">
      <div class="stat">
        <div class="stat-title">Total</div>
        <div class="stat-value text-lg">{data.total}</div>
      </div>
      <div class="stat">
        <div class="stat-title">Healthy</div>
        <div class="stat-value text-lg text-success">{data.healthy}</div>
      </div>
      <div class="stat">
        <div class="stat-title">Warning</div>
        <div class="stat-value text-lg text-warning">{data.warning}</div>
      </div>
      <div class="stat">
        <div class="stat-title">Error</div>
        <div class="stat-value text-lg text-error">{data.error}</div>
      </div>
    </div>

    <!-- Filter bar -->
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Search by name, UUID, switch..."
        class="input input-sm input-bordered w-72"
      />
      <div class="join">
        <button
          class="btn join-item btn-xs {severityFilter === 'all'
            ? 'btn-active'
            : ''}"
          onclick={() => (severityFilter = 'all')}>All</button
        >
        <button
          class="btn join-item btn-xs {severityFilter === 'error'
            ? 'btn-active'
            : ''}"
          onclick={() => (severityFilter = 'error')}>Errors</button
        >
        <button
          class="btn join-item btn-xs {severityFilter === 'warning'
            ? 'btn-active'
            : ''}"
          onclick={() => (severityFilter = 'warning')}>Warnings</button
        >
        <button
          class="btn join-item btn-xs {severityFilter === 'healthy'
            ? 'btn-active'
            : ''}"
          onclick={() => (severityFilter = 'healthy')}>Healthy</button
        >
      </div>
      <span class="text-sm text-base-content/50"
        >{filteredPorts.length} ports</span
      >
    </div>

    <!-- Port list -->
    <div class="flex flex-col gap-2">
      {#each filteredPorts as port (port.port_uuid)}
        <div
          class="rounded-lg border-l-4 bg-base-100 shadow-sm {severityColor(
            port.overall,
          )}"
        >
          <button
            type="button"
            class="flex w-full cursor-pointer items-center justify-between px-4 py-3 text-left hover:bg-base-200"
            onclick={() => togglePort(port.port_uuid)}
          >
            <div class="flex items-center gap-3">
              <span class="badge badge-sm {severityBadge(port.overall)}"
                >{port.overall}</span
              >
              <div>
                <span class="font-semibold">{port.port_name}</span>
                {#if port.port_type}
                  <span class="badge badge-ghost badge-xs ml-1"
                    >{port.port_type}</span
                  >
                {/if}
                {#if port.switch_name}
                  <span class="ml-2 text-xs text-base-content/50"
                    >on {port.switch_name}</span
                  >
                {/if}
              </div>
            </div>
            <div class="flex items-center gap-2">
              <a
                href={link(
                  `/correlated/logical-switch-ports/${port.port_uuid}`,
                )}
                class="btn btn-ghost btn-xs"
                onclick={(e) => e.stopPropagation()}>View</a
              >
              <span class="text-xs text-base-content/40"
                >{expandedPorts.has(port.port_uuid) ? '-' : '+'}</span
              >
            </div>
          </button>

          {#if expandedPorts.has(port.port_uuid)}
            <div class="border-t border-base-300 px-4 py-3">
              <div class="space-y-1">
                {#each port.checks as check}
                  <div class="flex items-start gap-2 text-sm">
                    <span
                      class="badge badge-xs mt-0.5 {checkStatusBadge(
                        check.status,
                      )}"
                    ></span>
                    <span class="font-mono text-xs text-base-content/60"
                      >{check.name}</span
                    >
                    <span class="text-xs">{check.message}</span>
                  </div>
                {/each}
              </div>
              <div class="mt-2 text-xs text-base-content/40">
                UUID: {port.port_uuid}
              </div>
            </div>
          {/if}
        </div>
      {/each}

      {#if filteredPorts.length === 0}
        <div class="py-8 text-center text-sm text-base-content/40">
          No ports match the current filter
        </div>
      {/if}
    </div>
  {/if}
</div>
