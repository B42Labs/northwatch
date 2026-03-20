<script lang="ts">
  import type { FlowEntry } from '../../lib/api';
  import { SvelteSet } from 'svelte/reactivity';

  import { link } from '../../lib/router';

  let {
    tableId,
    tableName = '',
    flows,
    pipeline = 'ingress',
    searchQuery = '',
  }: {
    tableId: number;
    tableName?: string;
    flows: FlowEntry[];
    pipeline?: string;
    searchQuery?: string;
  } = $props();

  let expandedFlows = new SvelteSet<string>();
  let collapsed = $state(false);

  function toggleFlow(uuid: string) {
    if (expandedFlows.has(uuid)) {
      expandedFlows.delete(uuid);
    } else {
      expandedFlows.add(uuid);
    }
  }

  function matchesSearch(flow: FlowEntry, q: string): boolean {
    if (!q) return true;
    const lower = q.toLowerCase();
    return (
      flow.match.toLowerCase().includes(lower) ||
      flow.actions.toLowerCase().includes(lower) ||
      String(flow.priority).includes(lower) ||
      flow.uuid.toLowerCase().includes(lower)
    );
  }

  let filteredFlows = $derived(
    searchQuery ? flows.filter((f) => matchesSearch(f, searchQuery)) : flows,
  );

  let matchCount = $derived(filteredFlows.length);
  let hasSearch = $derived(searchQuery.length > 0);

  // Categorize common OVN actions for visual hints
  function actionBadges(actions: string): { label: string; cls: string }[] {
    const badges: { label: string; cls: string }[] = [];
    if (actions.includes('drop'))
      badges.push({ label: 'drop', cls: 'badge-error' });
    if (actions.includes('ct_next'))
      badges.push({ label: 'CT', cls: 'badge-warning' });
    if (actions.includes('ct_snat'))
      badges.push({ label: 'SNAT', cls: 'badge-info' });
    if (actions.includes('ct_dnat'))
      badges.push({ label: 'DNAT', cls: 'badge-info' });
    if (actions.includes('ct_commit'))
      badges.push({ label: 'commit', cls: 'badge-warning' });
    if (/(?<![a-z_])next(?![a-z_])/.test(actions))
      badges.push({ label: 'next', cls: 'badge-ghost' });
    if (actions.includes('output'))
      badges.push({ label: 'output', cls: 'badge-success' });
    if (actions.includes('arp') || actions.includes('nd_na'))
      badges.push({ label: 'ARP/ND', cls: 'badge-accent' });
    if (actions.includes('icmp'))
      badges.push({ label: 'ICMP', cls: 'badge-accent' });
    return badges;
  }
</script>

<div class="rounded-lg border border-base-300 bg-base-100">
  <!-- Table header -->
  <button
    type="button"
    class="flex w-full cursor-pointer items-center justify-between border-b border-base-300 px-3 py-2 hover:bg-base-200 {pipeline ===
    'ingress'
      ? 'bg-info/5'
      : 'bg-warning/5'}"
    onclick={() => (collapsed = !collapsed)}
  >
    <div class="flex items-center gap-2">
      <span
        class="text-xs text-base-content/40 transition-transform {collapsed
          ? ''
          : 'rotate-90'}">&#9654;</span
      >
      <span class="text-sm font-semibold"
        >Table {tableId}{#if tableName}
          <span class="ml-1 font-normal text-base-content/60">{tableName}</span
          >{/if}</span
      >
      <span
        class="badge badge-sm {hasSearch && matchCount === 0
          ? 'badge-ghost'
          : pipeline === 'ingress'
            ? 'badge-info'
            : 'badge-warning'} badge-outline"
      >
        {#if hasSearch}
          {matchCount}/{flows.length}
        {:else}
          {flows.length}
        {/if}
      </span>
    </div>
  </button>

  {#if !collapsed}
    <div class="max-h-[500px] overflow-y-auto">
      {#if filteredFlows.length === 0}
        <div class="px-3 py-4 text-center text-xs text-base-content/40">
          {#if hasSearch}
            No flows matching "{searchQuery}"
          {:else}
            No flows
          {/if}
        </div>
      {:else}
        {#each filteredFlows as flow, i}
          <button
            type="button"
            class="block w-full cursor-pointer border-b border-base-300 px-3 py-2 text-left text-xs last:border-0 hover:bg-base-200 {i %
              2 ===
            0
              ? 'bg-base-100'
              : 'bg-base-200/30'}"
            onclick={() => toggleFlow(flow.uuid)}
          >
            <!-- Flow summary row -->
            <div class="flex items-start gap-2">
              <span class="badge badge-ghost badge-sm mt-0.5 shrink-0 font-mono"
                >{flow.priority}</span
              >
              <div class="min-w-0 flex-1">
                <div
                  class="break-all font-mono leading-relaxed text-base-content/80"
                >
                  {flow.match || '1 (any)'}
                </div>
                {#if !expandedFlows.has(flow.uuid)}
                  <div class="mt-1 flex flex-wrap gap-1">
                    {#each actionBadges(flow.actions) as badge}
                      <span class="badge badge-xs {badge.cls}"
                        >{badge.label}</span
                      >
                    {/each}
                  </div>
                {/if}
              </div>
            </div>

            <!-- Expanded details -->
            {#if expandedFlows.has(flow.uuid)}
              <div
                class="mt-2 space-y-2 rounded border border-base-300 bg-base-200 p-3"
              >
                <div>
                  <div class="mb-0.5 font-semibold text-base-content/50">
                    Match
                  </div>
                  <div class="break-all font-mono leading-relaxed">
                    {flow.match || '1 (any)'}
                  </div>
                </div>
                <div>
                  <div
                    class="mb-0.5 flex items-center gap-2 font-semibold text-base-content/50"
                  >
                    Actions
                    {#each actionBadges(flow.actions) as badge}
                      <span class="badge badge-xs {badge.cls}"
                        >{badge.label}</span
                      >
                    {/each}
                  </div>
                  <div class="break-all font-mono leading-relaxed">
                    {flow.actions}
                  </div>
                </div>
                {#if flow.external_ids && Object.keys(flow.external_ids).length > 0}
                  <div>
                    <div class="mb-0.5 font-semibold text-base-content/50">
                      External IDs
                    </div>
                    <div class="flex flex-wrap gap-1">
                      {#each Object.entries(flow.external_ids) as [key, value]}
                        {#if key === 'source' && /^[0-9a-f-]{36}$/i.test(value)}
                          <a
                            href={link(`/nb/acls/${value}`)}
                            class="badge badge-primary badge-outline badge-sm gap-1"
                            onclick={(e) => e.stopPropagation()}
                          >
                            {key}: {value.slice(0, 8)}...
                          </a>
                        {:else}
                          <span class="badge badge-ghost badge-outline badge-sm"
                            >{key}: {value}</span
                          >
                        {/if}
                      {/each}
                    </div>
                  </div>
                {/if}
                <div
                  class="flex gap-4 border-t border-base-300 pt-1 text-base-content/40"
                >
                  <span
                    >Priority: <span class="font-mono">{flow.priority}</span
                    ></span
                  >
                  <span>UUID: <span class="font-mono">{flow.uuid}</span></span>
                </div>
              </div>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>
