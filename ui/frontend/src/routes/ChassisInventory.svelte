<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet, SvelteMap } from 'svelte/reactivity';
  import {
    listChassisInventory,
    getChassisInventory,
    listOvsMembers,
    type ChassisInventoryEntry,
    type ChassisInventoryDetail,
    type ChassisLiveness,
  } from '../lib/api';
  import type { Variant } from '../lib/status';
  import { link } from '../lib/router';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import KeyValueTable from '../components/ui/KeyValueTable.svelte';

  // chassis auto-expands the matching row when arriving from the OVS view via
  // ?chassis=<system-id>, so the two views of the same node line up.
  let { chassis = '' }: { chassis?: string } = $props();

  let data: ChassisInventoryEntry[] | null = $state(null);
  let loading = $state(true);
  let error = $state('');

  // system-ids that have a live OVS pool connection, so their row can link to
  // the OVS view. Best-effort — empty when OVS visibility is disabled.
  let ovsMembers = new SvelteSet<string>();

  let livenessFilter = $state('all');
  const filterOptions = [
    { value: 'all', label: 'All' },
    { value: 'alive', label: 'Alive' },
    { value: 'down', label: 'Down' },
    { value: 'lagging', label: 'Out-of-sync' },
  ];

  // Expanded rows lazy-load the per-chassis detail (other_config + bound ports).
  let expanded = new SvelteSet<string>();
  type DetailState =
    | { status: 'loading' }
    | { status: 'loaded'; detail: ChassisInventoryDetail }
    | { status: 'error'; error: string };
  let details = new SvelteMap<string, DetailState>();

  async function load() {
    loading = true;
    error = '';
    try {
      data = await listChassisInventory();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  async function toggle(name: string) {
    if (expanded.has(name)) {
      expanded.delete(name);
      return;
    }
    expanded.add(name);
    if (details.has(name)) return;
    details.set(name, { status: 'loading' });
    try {
      const detail = await getChassisInventory(name);
      details.set(name, { status: 'loaded', detail });
    } catch (e) {
      details.set(name, {
        status: 'error',
        error: e instanceof Error ? e.message : 'Failed to load detail',
      });
    }
  }

  async function refresh() {
    // Drop cached details so a reopened row refetches against fresh data.
    details.clear();
    expanded.clear();
    await load();
  }

  let rows: ChassisInventoryEntry[] = $derived(data ?? []);

  let filtered = $derived(
    rows.filter((c) => {
      switch (livenessFilter) {
        case 'alive':
          return c.liveness.alive;
        case 'down':
          return !c.liveness.alive;
        case 'lagging':
          return !c.liveness.in_sync;
        default:
          return true;
      }
    }),
  );

  let summary = $derived.by(() => {
    const list = rows;
    const alive = list.filter((c) => c.liveness.alive).length;
    const synced = list.filter((c) => c.liveness.in_sync).length;
    const ports = list.reduce((n, c) => n + c.ports.total, 0);
    return {
      total: list.length,
      alive,
      synced,
      down: list.length - alive,
      ports,
    };
  });

  // age_ms is the heartbeat age; 0 means "no heartbeat" (no Chassis_Private row).
  function formatAge(ms: number): string {
    if (!ms || ms <= 0) return '—';
    const s = Math.floor(ms / 1000);
    if (s < 60) return `${s}s`;
    const m = Math.floor(s / 60);
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h`;
    const d = Math.floor(h / 24);
    return `${d}d ${h % 24}h`;
  }

  function syncBadge(l: ChassisLiveness): { text: string; variant: Variant } {
    if (l.in_sync) return { text: 'in-sync', variant: 'info' };
    // A lagging chassis is a soft warning until it crosses the stale threshold,
    // at which point it is flagged as stuck (error).
    const variant: Variant = l.stale ? 'error' : 'warning';
    return { text: `lag ${l.nb_cfg}/${l.sb_nb_cfg}`, variant };
  }

  async function loadOvsMembers() {
    // Best-effort: OVS visibility may be disabled (404) or unavailable in
    // snapshot/multi-cluster mode, in which case no OVS links are shown.
    const members = await listOvsMembers().catch(() => []);
    for (const m of members) ovsMembers.add(m.system_id);
  }

  onMount(async () => {
    await Promise.all([load(), loadOvsMembers()]);
    // Auto-expand the chassis deep-linked from the OVS view.
    if (chassis && (data ?? []).some((c) => c.name === chassis)) {
      toggle(chassis);
    }
  });
</script>

<PageHeader
  eyebrow="Monitoring"
  title="Chassis Inventory"
  description="Aggregated Southbound view of every chassis: tunnel encapsulation, nb_cfg-derived liveness, and the logical-port workload bound to each node."
>
  {#snippet actions()}
    <button class="btn btn-ghost btn-xs border-base-300" onclick={refresh}>
      Refresh
    </button>
  {/snippet}
</PageHeader>

<DataState {loading} {error} empty={!data} emptyMessage="no chassis">
  {#if data}
    <StatTiles
      class="mb-4 w-full"
      tiles={[
        { label: 'Chassis', value: summary.total },
        { label: 'Alive', value: summary.alive, variant: 'success' },
        { label: 'In-sync', value: summary.synced, variant: 'info' },
        {
          label: 'Down',
          value: summary.down,
          variant: summary.down > 0 ? 'warning' : 'neutral',
        },
        { label: 'Bound Ports', value: summary.ports },
      ]}
    />

    <div class="mb-4 flex items-center gap-3">
      <SegmentedControl
        options={filterOptions}
        bind:value={livenessFilter}
        size="xs"
      />
      <span class="font-mono text-xs text-base-content/55"
        >{filtered.length} chassis</span
      >
    </div>

    <div class="overflow-x-auto rounded border border-base-300">
      <table class="table table-xs font-mono">
        <thead>
          <tr>
            {#each ['Chassis', 'Hostname', 'Liveness', 'Ports', 'Encaps', 'Bridge mappings'] as h (h)}
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >{h}</th
              >
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each filtered as c (c.name)}
            {@const enc = c.encaps ?? []}
            <tr class="border-base-300/60">
              <td>
                <div class="flex items-center gap-2">
                  <button
                    class="flex items-center gap-1.5 text-left transition-colors hover:text-primary"
                    onclick={() => toggle(c.name)}
                    aria-expanded={expanded.has(c.name)}
                  >
                    <span
                      class="select-none text-base-content/40"
                      aria-hidden="true"
                      >{expanded.has(c.name) ? '▾' : '▸'}</span
                    >
                    <span class="break-all text-xs">{c.name}</span>
                  </button>
                  {#if ovsMembers.has(c.name)}
                    <a
                      class="font-mono text-2xs text-primary hover:underline"
                      href={link(`/ovs?chassis=${encodeURIComponent(c.name)}`)}
                      >OVS →</a
                    >
                  {/if}
                </div>
              </td>
              <td class="text-xs text-base-content/70">{c.hostname || '—'}</td>
              <td>
                <div class="flex flex-wrap items-center gap-1">
                  <Badge
                    text={c.liveness.alive ? 'alive' : 'down'}
                    variant={c.liveness.alive ? 'success' : 'error'}
                    glyph={c.liveness.alive ? '●' : '○'}
                  />
                  <Badge {...syncBadge(c.liveness)} />
                  <span class="text-2xs text-base-content/45"
                    >{formatAge(c.liveness.age_ms)}</span
                  >
                </div>
              </td>
              <td>
                <div class="flex flex-wrap items-center gap-1">
                  <span
                    class="text-xs {c.ports.total === 0
                      ? 'text-base-content/40'
                      : ''}">{c.ports.up}/{c.ports.total} up</span
                  >
                  {#each Object.entries(c.ports.by_type) as [t, n] (t)}
                    <span class="badge badge-ghost badge-xs">{t} {n}</span>
                  {/each}
                </div>
              </td>
              <td>
                {#if enc.length === 0}
                  <span class="text-2xs text-base-content/40">—</span>
                {:else}
                  <div class="flex flex-col gap-0.5">
                    {#each enc as e (e.type + e.ip)}
                      <span class="whitespace-nowrap text-xs">
                        <span class="text-base-content/50">{e.type}</span>
                        {e.ip}
                      </span>
                    {/each}
                  </div>
                {/if}
              </td>
              <td
                class="max-w-xs truncate text-xs text-base-content/60"
                title={c.bridge_mappings}>{c.bridge_mappings || '—'}</td
              >
            </tr>

            {#if expanded.has(c.name)}
              {@const d = details.get(c.name)}
              <tr class="border-base-300/60 bg-base-200/30">
                <td colspan="6" class="p-3">
                  {#if !d || d.status === 'loading'}
                    <div class="flex items-center gap-2">
                      <span class="loading loading-spinner loading-xs"></span>
                      <span class="text-xs text-base-content/55"
                        >loading detail…</span
                      >
                    </div>
                  {:else if d.status === 'error'}
                    <div class="text-xs text-error">
                      <span class="text-base-content/30">//</span>
                      {d.error}
                    </div>
                  {:else}
                    {@const detail = d.detail}
                    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                      <div>
                        <div
                          class="mb-1.5 text-2xs uppercase tracking-wider text-base-content/45"
                        >
                          Other config
                        </div>
                        <KeyValueTable data={detail.other_config ?? {}} />
                      </div>
                      <div>
                        <div
                          class="mb-1.5 text-2xs uppercase tracking-wider text-base-content/45"
                        >
                          Bound ports ({detail.bound_ports.length})
                        </div>
                        {#if detail.bound_ports.length === 0}
                          <span class="text-xs italic text-base-content/45"
                            >no ports bound</span
                          >
                        {:else}
                          <div
                            class="overflow-x-auto rounded border border-base-300"
                          >
                            <table class="table table-xs font-mono">
                              <thead>
                                <tr>
                                  {#each ['Logical port', 'Type', 'Up', 'Tunnel key'] as h (h)}
                                    <th
                                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                                      >{h}</th
                                    >
                                  {/each}
                                </tr>
                              </thead>
                              <tbody>
                                {#each detail.bound_ports as p (p.logical_port)}
                                  <tr class="border-base-300/60">
                                    <td class="break-all text-xs"
                                      >{p.logical_port}</td
                                    >
                                    <td class="text-xs text-base-content/70"
                                      >{p.type || 'vif'}</td
                                    >
                                    <td>
                                      {#if p.up === undefined}
                                        <span class="text-base-content/40"
                                          >—</span
                                        >
                                      {:else}
                                        <Badge
                                          text={p.up ? 'up' : 'down'}
                                          variant={p.up ? 'success' : 'neutral'}
                                          glyph={p.up ? '●' : '○'}
                                        />
                                      {/if}
                                    </td>
                                    <td class="text-xs text-base-content/70"
                                      >{p.tunnel_key}</td
                                    >
                                  </tr>
                                {/each}
                              </tbody>
                            </table>
                          </div>
                        {/if}
                      </div>
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>

    {#if filtered.length === 0}
      <div class="py-8 text-center">
        <span class="font-mono text-sm text-base-content/40"
          ><span class="text-base-content/30">//</span>
          {summary.total === 0
            ? 'no chassis registered in the Southbound database'
            : 'no chassis match the current filter'}</span
        >
      </div>
    {/if}
  {/if}
</DataState>
