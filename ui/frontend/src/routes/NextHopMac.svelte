<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { get as apiGet } from '../lib/api';
  import { writeEnabled } from '../lib/capabilitiesStore';
  import {
    previewOperations,
    applyPlan,
    cancelPlan,
    type Plan,
    type WriteOperation,
  } from '../lib/writeApi';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import FormField from '../components/ui/FormField.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import { type Variant } from '../lib/status';
  import PlanDiffView from '../components/write/PlanDiffView.svelte';

  interface NextHop {
    router_uuid: string;
    router_name: string;
    route_uuid: string;
    ip_prefix: string;
    route_table?: string;
    nexthop: string;
    lrp_name?: string;
    cached_mac?: string;
    mac_binding_uuid?: string;
    static_mac?: string;
    override_dynamic_mac?: boolean;
    aging_enabled: boolean;
    age_threshold_seconds?: number;
    age_seconds?: number;
    has_timestamp: boolean;
    status: string;
    overall: string;
  }

  interface Report {
    total: number;
    healthy: number;
    warning: number;
    error: number;
    no_aging: number;
    stale: number;
    mac_conflict: number;
    unresolved: number;
    next_hops: NextHop[];
  }

  let data = $state<Report | null>(null);
  let loading = $state(true);
  let error = $state('');
  let statusFilter = $state('all');

  const filterOptions = [
    { value: 'all', label: 'All' },
    { value: 'no-aging', label: 'No Aging' },
    { value: 'stale', label: 'Stale' },
    { value: 'mac-conflict', label: 'Conflict' },
    { value: 'unresolved', label: 'Unresolved' },
  ];

  // Selection keyed by route_uuid; only rows with a dynamic MAC_Binding can be
  // destroyed (you can only invalidate a cache entry that actually exists).
  let selected = new SvelteSet<string>();

  let deleteStep = $state<
    'idle' | 'preview' | 'confirming' | 'applying' | 'done' | 'error'
  >('idle');
  let plan = $state<Plan | null>(null);
  let deleteError = $state('');
  let actor = $state('');

  function setStatusFilter(val: string) {
    statusFilter = val;
    selected.clear();
  }

  let filtered = $derived(
    (data?.next_hops ?? []).filter(
      (h) => statusFilter === 'all' || h.status === statusFilter,
    ),
  );

  // Rows that carry a destroyable dynamic binding.
  let selectable = $derived(filtered.filter((h) => !!h.mac_binding_uuid));
  let allSelectableSelected = $derived(
    selectable.length > 0 &&
      selectable.every((h) => selected.has(h.route_uuid)),
  );

  function statusVariant(status: string): Variant {
    switch (status) {
      case 'ok':
      case 'pinned':
        return 'success';
      case 'no-aging':
      case 'stale':
      case 'mac-conflict':
        return 'warning';
      case 'unresolved':
        return 'neutral';
      default:
        return 'neutral';
    }
  }

  function formatAge(h: NextHop): string {
    if (!h.cached_mac) return '';
    if (!h.has_timestamp) return 'unknown';
    const s = h.age_seconds ?? 0;
    if (s < 60) return `${s}s`;
    const m = Math.floor(s / 60);
    if (m < 60) return `${m}m`;
    const hours = Math.floor(m / 60);
    if (hours < 24) return `${hours}h ${m % 60}m`;
    return `${Math.floor(hours / 24)}d ${hours % 24}h`;
  }

  function agingLabel(h: NextHop): string {
    return h.aging_enabled ? `${h.age_threshold_seconds}s` : 'off';
  }

  function toggleSelect(routeUUID: string) {
    if (selected.has(routeUUID)) selected.delete(routeUUID);
    else selected.add(routeUUID);
  }

  function toggleSelectAll() {
    if (allSelectableSelected) {
      for (const h of selectable) selected.delete(h.route_uuid);
    } else {
      for (const h of selectable) selected.add(h.route_uuid);
    }
  }

  // Map the selected rows to the unique MAC_Binding UUIDs to destroy. A single
  // binding can back several routes, so dedupe before building operations.
  function selectedBindings(): { uuid: string; reason: string }[] {
    const seen: Record<string, true> = {};
    const out: { uuid: string; reason: string }[] = [];
    for (const h of selectable) {
      if (!selected.has(h.route_uuid) || !h.mac_binding_uuid) continue;
      if (seen[h.mac_binding_uuid]) continue;
      seen[h.mac_binding_uuid] = true;
      out.push({
        uuid: h.mac_binding_uuid,
        reason: `Invalidate stale next-hop MAC ${h.cached_mac} for ${h.nexthop} on ${h.router_name} (router re-ARPs on next use)`,
      });
    }
    return out;
  }

  async function startDelete() {
    const bindings = selectedBindings();
    if (bindings.length === 0) return;

    deleteStep = 'preview';
    deleteError = '';
    plan = null;

    const operations: WriteOperation[] = bindings.map((b) => ({
      action: 'delete' as const,
      table: 'MAC_Binding',
      uuid: b.uuid,
      reason: b.reason,
    }));

    try {
      plan = await previewOperations(operations, 'Next-hop MAC cleanup');
      deleteStep = 'confirming';
    } catch (e) {
      deleteError = e instanceof Error ? e.message : 'Preview failed';
      deleteStep = 'error';
    }
  }

  async function confirmDelete() {
    if (!plan) return;
    deleteStep = 'applying';
    try {
      await applyPlan(plan.id, plan.apply_token, actor || undefined);
      deleteStep = 'done';
      selected.clear();
      await load();
    } catch (e) {
      deleteError = e instanceof Error ? e.message : 'Apply failed';
      deleteStep = 'error';
    }
  }

  async function cancelDelete() {
    if (plan && deleteStep === 'confirming') {
      try {
        await cancelPlan(plan.id);
      } catch {
        // ignore cancel errors
      }
    }
    resetDeleteFlow();
  }

  function resetDeleteFlow() {
    deleteStep = 'idle';
    plan = null;
    deleteError = '';
    actor = '';
  }

  async function load() {
    loading = true;
    error = '';
    try {
      data = await apiGet<Report>('/api/v1/debug/nexthop-mac');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  let expired = $state(false);
  $effect(() => {
    if (!plan?.expires_at) {
      expired = false;
      return;
    }
    const check = () => {
      expired = Date.now() >= new Date(plan!.expires_at).getTime();
    };
    check();
    const interval = setInterval(check, 1000);
    return () => clearInterval(interval);
  });

  onMount(() => load());
</script>

<PageHeader
  eyebrow="Debug"
  title="Next-Hop MAC"
  description="Static-route next hops correlated with their cached SB MAC_Binding. Flags learned next-hop MACs that can go stale because ARP-cache aging (mac_binding_age_threshold) is not configured — a changed next-hop MAC would not be refreshed."
/>

<DataState {loading} {error} empty={!data} emptyMessage="no data">
  {#if data}
    <StatTiles
      class="mb-4 w-full"
      tiles={[
        { label: 'Next Hops', value: data.total },
        { label: 'No Aging', value: data.no_aging, variant: 'warning' },
        { label: 'Stale', value: data.stale, variant: 'warning' },
        { label: 'Conflicts', value: data.mac_conflict, variant: 'warning' },
        { label: 'Unresolved', value: data.unresolved },
      ]}
    />

    <div class="mb-4 flex items-center gap-3">
      <SegmentedControl
        options={filterOptions}
        bind:value={statusFilter}
        onchange={setStatusFilter}
        size="xs"
      />
      <span class="font-mono text-xs text-base-content/55"
        >{filtered.length} next hops</span
      >

      {#if $writeEnabled && selected.size > 0 && deleteStep === 'idle'}
        <button class="btn ml-auto btn-error btn-xs" onclick={startDelete}>
          Invalidate {selectedBindings().length} MAC binding{selectedBindings()
            .length === 1
            ? ''
            : 's'}
        </button>
      {/if}
    </div>

    {#if deleteStep !== 'idle'}
      <div class="mb-4 rounded border border-base-300 bg-base-100 p-4">
        {#if deleteStep === 'preview'}
          <div class="flex items-center gap-2">
            <span class="loading loading-sm loading-spinner"></span>
            <span class="font-mono text-sm">Previewing changes...</span>
          </div>
        {:else if deleteStep === 'confirming' && plan}
          <div class="flex flex-col gap-3">
            <h3
              class="font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
            >
              Confirm — destroy {plan.operations.length} MAC_Binding entr{plan
                .operations.length === 1
                ? 'y'
                : 'ies'}
            </h3>
            <p class="font-mono text-2xs text-base-content/55">
              The logical router re-sends ARP/ND and repopulates the binding on
              the next packet.
            </p>

            <PlanDiffView diffs={plan.diffs} />

            <div
              class="flex flex-wrap items-end gap-3 border-t border-base-300 pt-3"
            >
              <FormField label="Actor (optional)" forId="nexthop-actor">
                <input
                  id="nexthop-actor"
                  type="text"
                  class="input w-48 font-mono input-sm"
                  placeholder="your-name"
                  bind:value={actor}
                />
              </FormField>
              <button
                class="btn btn-error btn-sm"
                disabled={expired}
                onclick={confirmDelete}
              >
                Confirm
              </button>
              <button
                class="btn border-base-300 btn-ghost btn-sm"
                onclick={cancelDelete}
              >
                Cancel
              </button>
            </div>

            {#if expired}
              <div role="alert" class="alert py-2 text-xs alert-error">
                Plan has expired. Cancel and try again.
              </div>
            {/if}
          </div>
        {:else if deleteStep === 'applying'}
          <div class="flex items-center gap-2">
            <span class="loading loading-sm loading-spinner"></span>
            <span class="font-mono text-sm">Destroying bindings...</span>
          </div>
        {:else if deleteStep === 'done'}
          <div role="alert" class="alert py-2 alert-success">
            <span class="text-sm">MAC bindings invalidated.</span>
            <button
              class="btn border-base-300 btn-ghost btn-xs"
              onclick={resetDeleteFlow}
            >
              Dismiss
            </button>
          </div>
        {:else if deleteStep === 'error'}
          <div role="alert" class="alert py-2 alert-error">
            <span class="text-sm">Error: {deleteError}</span>
            <button
              class="btn border-base-300 btn-ghost btn-xs"
              onclick={resetDeleteFlow}
            >
              Dismiss
            </button>
          </div>
        {/if}
      </div>
    {/if}

    <div class="overflow-x-auto rounded border border-base-300">
      <table class="table table-xs font-mono">
        <thead>
          <tr>
            {#if $writeEnabled}
              <th
                class="w-8 bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >
                <input
                  type="checkbox"
                  class="checkbox checkbox-xs"
                  checked={allSelectableSelected}
                  onchange={toggleSelectAll}
                  disabled={deleteStep !== 'idle' || selectable.length === 0}
                />
              </th>
            {/if}
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Status</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Router</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Next Hop</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Egress LRP</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Cached MAC</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Static MAC</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Age</th
            >
            <th
              class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
              >Aging</th
            >
          </tr>
        </thead>
        <tbody>
          {#each filtered as hop (hop.route_uuid)}
            <tr
              class="border-base-300/60 {selected.has(hop.route_uuid)
                ? 'bg-base-300/40'
                : ''}"
            >
              {#if $writeEnabled}
                <td>
                  {#if hop.mac_binding_uuid}
                    <input
                      type="checkbox"
                      class="checkbox checkbox-xs"
                      checked={selected.has(hop.route_uuid)}
                      onchange={() => toggleSelect(hop.route_uuid)}
                      disabled={deleteStep !== 'idle'}
                    />
                  {/if}
                </td>
              {/if}
              <td
                ><Badge
                  text={hop.status}
                  variant={statusVariant(hop.status)}
                /></td
              >
              <td class="text-xs">{hop.router_name}</td>
              <td class="text-xs">
                {hop.nexthop}
                <span class="text-base-content/40">via {hop.ip_prefix}</span>
              </td>
              <td class="text-xs">{hop.lrp_name ?? ''}</td>
              <td class="text-xs">{hop.cached_mac ?? '—'}</td>
              <td class="text-xs">
                {hop.static_mac ?? '—'}
                {#if hop.static_mac && hop.override_dynamic_mac}
                  <span class="text-base-content/40">(override)</span>
                {/if}
              </td>
              <td class="text-xs text-base-content/50">{formatAge(hop)}</td>
              <td class="text-xs">
                <span
                  class={hop.aging_enabled
                    ? 'text-base-content/50'
                    : 'text-warning'}>{agingLabel(hop)}</span
                >
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if filtered.length === 0}
      <div class="py-8 text-center">
        <span class="font-mono text-sm text-base-content/40"
          ><span class="text-base-content/30">//</span>
          {data.total === 0
            ? 'no static-route next hops found'
            : 'no next hops match the current filter'}</span
        >
      </div>
    {/if}
  {/if}
</DataState>
