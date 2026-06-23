<script lang="ts">
  import { onMount } from 'svelte';
  import { getPortDiagnostics, type PortDiagnosticsSummary } from '../lib/api';
  import { link } from '../lib/router';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import { severityVariant, accentBorderClass } from '../lib/status';
  import { SvelteSet } from 'svelte/reactivity';

  let data: PortDiagnosticsSummary | null = $state(null);
  let loading = $state(true);
  let error = $state('');
  let searchQuery = $state('');
  let severityFilter = $state('all');

  let expandedPorts = new SvelteSet<string>();

  const severityOptions = [
    { value: 'all', label: 'All' },
    { value: 'error', label: 'Errors' },
    { value: 'warning', label: 'Warnings' },
    { value: 'healthy', label: 'Healthy' },
  ];

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

<PageHeader
  eyebrow="Debug"
  title="Port Diagnostics"
  description="Health analysis of all logical ports: binding status, chassis health, type verification"
/>

<DataState {loading} {error} empty={!data} emptyMessage="no diagnostics">
  {#if data}
    <!-- Summary stats -->
    <StatTiles
      class="mb-4 w-full"
      tiles={[
        { label: 'Total', value: data.total },
        { label: 'Healthy', value: data.healthy, variant: 'success' },
        { label: 'Warning', value: data.warning, variant: 'warning' },
        { label: 'Error', value: data.error, variant: 'error' },
      ]}
    />

    <!-- Filter bar -->
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <FilterInput
        bind:value={searchQuery}
        placeholder="Search by name, UUID, switch..."
        width="w-72"
      />
      <SegmentedControl
        options={severityOptions}
        bind:value={severityFilter}
        size="xs"
      />
      <span class="font-mono text-xs text-base-content/55"
        >{filteredPorts.length} ports</span
      >
    </div>

    <!-- Port list -->
    <div class="flex flex-col gap-2">
      {#each filteredPorts as port (port.port_uuid)}
        <div
          class="rounded border border-l-2 border-base-300 bg-base-100 {accentBorderClass[
            severityVariant(port.overall)
          ]}"
        >
          <button
            type="button"
            class="flex w-full cursor-pointer items-center justify-between px-4 py-3 text-left hover:bg-base-300/40"
            onclick={() => togglePort(port.port_uuid)}
          >
            <div class="flex items-center gap-3">
              <Badge
                text={port.overall}
                variant={severityVariant(port.overall)}
              />
              <div>
                <span class="font-mono font-semibold">{port.port_name}</span>
                {#if port.port_type}
                  <span class="badge badge-ghost badge-xs ml-1"
                    >{port.port_type}</span
                  >
                {/if}
                {#if port.switch_name}
                  <span class="ml-2 font-mono text-xs text-base-content/50"
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
                class="btn btn-ghost btn-xs border-base-300"
                onclick={(e) => e.stopPropagation()}>View</a
              >
              <span class="font-mono text-xs text-base-content/40"
                >{expandedPorts.has(port.port_uuid) ? '-' : '+'}</span
              >
            </div>
          </button>

          {#if expandedPorts.has(port.port_uuid)}
            <div class="border-t border-base-300 px-4 py-3">
              <div class="space-y-1">
                {#each port.checks as check (check.name)}
                  <div class="flex items-start gap-2 font-mono text-sm">
                    <span
                      class="badge badge-xs mt-0.5 {severityVariant(
                        check.status,
                      ) === 'error'
                        ? 'badge-error'
                        : severityVariant(check.status) === 'warning'
                          ? 'badge-warning'
                          : 'badge-success'}"
                    ></span>
                    <span class="text-xs text-base-content/60"
                      >{check.name}</span
                    >
                    <span class="text-xs">{check.message}</span>
                  </div>
                {/each}
              </div>
              <div class="mt-2 font-mono text-xs text-base-content/40">
                UUID: {port.port_uuid}
              </div>
            </div>
          {/if}
        </div>
      {/each}

      {#if filteredPorts.length === 0}
        <div class="py-8 text-center">
          <span class="font-mono text-sm text-base-content/40"
            ><span class="text-base-content/30">//</span> no ports match the current
            filter</span
          >
        </div>
      {/if}
    </div>
  {/if}
</DataState>
