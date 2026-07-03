<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import {
    getGatewayHealth,
    type GatewayHealthReport,
    type GatewayHealth,
  } from '../../lib/api';
  import { subscribeToTables } from '../../lib/eventStore';
  import { link } from '../../lib/router';
  import { severityVariant, accentBorderClass } from '../../lib/status';
  import DataState from '../ui/DataState.svelte';
  import StatTiles from '../ui/StatTiles.svelte';
  import FilterInput from '../ui/FilterInput.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Badge from '../ui/Badge.svelte';
  import FailoverLadder from './FailoverLadder.svelte';

  let report = $state<GatewayHealthReport | null>(null);
  let loading = $state(true);
  let error = $state('');
  let searchQuery = $state('');
  let filter = $state('anomalies');
  let expanded = new SvelteSet<string>();
  let inFlight = false;

  const filterOptions = [
    { value: 'all', label: 'All' },
    { value: 'anomalies', label: 'Anomalies' },
    { value: 'error', label: 'Critical' },
    { value: 'warning', label: 'Warning' },
  ];

  function toggle(key: string) {
    if (expanded.has(key)) expanded.delete(key);
    else expanded.add(key);
  }

  function matchesSearch(gw: GatewayHealth, q: string): boolean {
    const hay = [
      gw.cr_port,
      gw.router_name ?? '',
      gw.ha_group_name ?? '',
      gw.desired_chassis ?? '',
      gw.actual_chassis ?? '',
      ...(gw.served_ips ?? []),
      ...(gw.external_networks ?? []),
      ...gw.members.map((m) => m.name),
    ]
      .join(' ')
      .toLowerCase();
    return hay.includes(q);
  }

  let filtered = $derived(
    (report?.gateways ?? []).filter((gw) => {
      if (filter === 'anomalies' && gw.overall === 'healthy') return false;
      if (filter === 'error' && gw.overall !== 'error') return false;
      if (filter === 'warning' && gw.overall !== 'warning') return false;
      if (searchQuery && !matchesSearch(gw, searchQuery.toLowerCase()))
        return false;
      return true;
    }),
  );

  async function load() {
    if (inFlight) return;
    inFlight = true;
    try {
      report = await getGatewayHealth();
      error = '';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load gateway health';
    } finally {
      loading = false;
      inFlight = false;
    }
  }

  onMount(load);

  $effect(() => {
    const unsub = subscribeToTables(
      '*',
      ['Port_Binding', 'Chassis', 'HA_Chassis', 'HA_Chassis_Group'],
      () => load(),
    );
    return () => unsub();
  });
</script>

<section class="mb-6">
  <div class="mb-2 flex items-baseline justify-between">
    <h2 class="font-mono text-sm font-semibold tracking-wide">
      Gateway Health
      <span class="ml-2 text-xs font-normal text-base-content/50"
        >desired vs. actual gateway ownership</span
      >
    </h2>
    <button
      class="btn border-base-300 btn-ghost font-mono btn-xs"
      onclick={() => load()}>refresh</button
    >
  </div>

  <DataState
    {loading}
    {error}
    empty={report !== null && report.total === 0}
    emptyMessage="no distributed gateway ports found"
  >
    {#if report}
      <StatTiles
        class="mb-3 w-full"
        tiles={[
          { label: 'Gateways', value: report.total },
          { label: 'Healthy', value: report.healthy, variant: 'success' },
          { label: 'Warning', value: report.warning, variant: 'warning' },
          { label: 'Critical', value: report.error, variant: 'error' },
        ]}
      />

      {#if report.conflicts && report.conflicts.length > 0}
        <div
          class="mb-3 rounded border border-l-2 border-base-300 border-l-error bg-base-100 px-4 py-3"
        >
          <div class="mb-1 font-mono text-xs font-semibold text-error">
            ⚠ Duplicate external IPs served from multiple chassis
          </div>
          <ul class="space-y-0.5">
            {#each report.conflicts as c (c.external_ip)}
              <li class="font-mono text-xs text-base-content/70">
                <span class="font-semibold text-base-content"
                  >{c.external_ip}</span
                >
                served by {c.chassis.join(', ')}
                <span class="text-base-content/40"
                  >({c.cr_ports.join(', ')})</span
                >
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      <div class="mb-3 flex flex-wrap items-center gap-3">
        <FilterInput
          bind:value={searchQuery}
          placeholder="Search IP, router, chassis…"
          width="w-72"
        />
        <SegmentedControl
          options={filterOptions}
          bind:value={filter}
          size="xs"
        />
        <span class="font-mono text-xs text-base-content/55"
          >{filtered.length} gateway{filtered.length === 1 ? '' : 's'}</span
        >
      </div>

      <div class="flex flex-col gap-2">
        {#each filtered as gw (gw.port_binding_uuid)}
          <div
            class="rounded border border-l-2 border-base-300 bg-base-100 {accentBorderClass[
              severityVariant(gw.overall)
            ]}"
          >
            <button
              type="button"
              class="flex w-full cursor-pointer items-center justify-between gap-3 px-4 py-3 text-left hover:bg-base-300/40"
              onclick={() => toggle(gw.port_binding_uuid)}
            >
              <div class="flex min-w-0 items-center gap-3">
                <Badge text={gw.status} variant={severityVariant(gw.overall)} />
                <div class="min-w-0">
                  <span class="font-mono font-semibold">{gw.cr_port}</span>
                  {#if gw.router_name}
                    <span class="ml-2 font-mono text-xs text-base-content/50"
                      >{gw.router_name}</span
                    >
                  {/if}
                  {#if gw.served_ips && gw.served_ips.length}
                    <div class="mt-0.5 flex flex-wrap gap-1">
                      {#each gw.served_ips as ip (ip)}
                        <span class="badge badge-ghost font-mono badge-xs"
                          >{ip}</span
                        >
                      {/each}
                    </div>
                  {/if}
                </div>
              </div>

              <div class="flex shrink-0 items-center gap-3">
                <span class="font-mono text-xs">
                  {#if !gw.actual_chassis}
                    <span class="text-error">no active chassis</span>
                  {:else if gw.desired_chassis && gw.desired_chassis !== gw.actual_chassis}
                    <span class="text-success">{gw.desired_chassis}</span>
                    <span class="px-1 text-error">↛</span>
                    <span class="text-error">{gw.actual_chassis}</span>
                  {:else}
                    <span class="text-base-content/45">on</span>
                    <span class="text-success">{gw.actual_chassis}</span>
                  {/if}
                </span>
                <span class="font-mono text-xs text-base-content/40"
                  >{expanded.has(gw.port_binding_uuid) ? '-' : '+'}</span
                >
              </div>
            </button>

            {#if expanded.has(gw.port_binding_uuid)}
              <div class="border-t border-base-300 px-4 py-3">
                <FailoverLadder gateway={gw} />

                <div class="mt-3 space-y-1">
                  {#each gw.checks as check (check.name)}
                    <div class="flex items-start gap-2 font-mono text-sm">
                      <span
                        class="mt-0.5 badge badge-xs {severityVariant(
                          check.status,
                        ) === 'error'
                          ? 'badge-error'
                          : severityVariant(check.status) === 'warning'
                            ? 'badge-warning'
                            : 'badge-success'}"
                      ></span>
                      <span class="w-40 shrink-0 text-xs text-base-content/55"
                        >{check.name}</span
                      >
                      <span class="text-xs">{check.message}</span>
                    </div>
                  {/each}
                </div>

                <div
                  class="mt-3 flex items-center gap-3 font-mono text-2xs text-base-content/40"
                >
                  <a
                    href={link(
                      `/correlated/port-bindings/${gw.port_binding_uuid}`,
                    )}
                    class="btn border-base-300 btn-ghost btn-xs"
                    onclick={(e) => e.stopPropagation()}>cr-port</a
                  >
                  {#if gw.router_uuid}
                    <a
                      href={link(
                        `/correlated/logical-routers/${gw.router_uuid}`,
                      )}
                      class="btn border-base-300 btn-ghost btn-xs"
                      onclick={(e) => e.stopPropagation()}>router</a
                    >
                  {/if}
                  {#if gw.ha_group_name}
                    <span>HA group: {gw.ha_group_name}</span>
                  {/if}
                </div>
              </div>
            {/if}
          </div>
        {/each}

        {#if filtered.length === 0}
          <div class="py-6 text-center">
            <span class="font-mono text-sm text-base-content/40"
              ><span class="text-base-content/30">//</span> no gateways match the
              current filter</span
            >
          </div>
        {/if}
      </div>
    {/if}
  </DataState>
</section>
