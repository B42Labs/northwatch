<script lang="ts">
  import type { FlowEntry } from '../../lib/api';
  import { SvelteSet } from 'svelte/reactivity';

  import { link } from '../../lib/router';
  import Badge from '../ui/Badge.svelte';
  import type { Variant } from '../../lib/status';

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
  function actionBadges(actions: string): { label: string; variant: Variant }[] {
    const badges: { label: string; variant: Variant }[] = [];
    if (actions.includes('drop'))
      badges.push({ label: 'drop', variant: 'error' });
    if (actions.includes('ct_next'))
      badges.push({ label: 'CT', variant: 'warning' });
    if (actions.includes('ct_snat'))
      badges.push({ label: 'SNAT', variant: 'info' });
    if (actions.includes('ct_dnat'))
      badges.push({ label: 'DNAT', variant: 'info' });
    if (actions.includes('ct_commit'))
      badges.push({ label: 'commit', variant: 'warning' });
    if (/(?<![a-z_])next(?![a-z_])/.test(actions))
      badges.push({ label: 'next', variant: 'ghost' });
    if (actions.includes('output'))
      badges.push({ label: 'output', variant: 'success' });
    if (actions.includes('arp') || actions.includes('nd_na'))
      badges.push({ label: 'ARP/ND', variant: 'accent' });
    if (actions.includes('icmp'))
      badges.push({ label: 'ICMP', variant: 'accent' });
    return badges;
  }
</script>

<div class="rounded border border-base-300 bg-base-100">
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
        class="select-none text-primary transition-transform {collapsed
          ? ''
          : 'rotate-90'}"
        aria-hidden="true">&#9654;</span
      >
      <span class="font-mono text-sm font-semibold"
        >Table {tableId}{#if tableName}
          <span class="ml-1 font-normal text-base-content/60">{tableName}</span
          >{/if}</span
      >
      <Badge
        text={hasSearch ? `${matchCount}/${flows.length}` : String(flows.length)}
        variant={hasSearch && matchCount === 0
          ? 'ghost'
          : pipeline === 'ingress'
            ? 'info'
            : 'warning'}
        outline
      />
    </div>
  </button>

  {#if !collapsed}
    <div class="max-h-[500px] overflow-y-auto">
      {#if filteredFlows.length === 0}
        <div class="px-3 py-4 text-center font-mono text-xs text-base-content/40">
          <span class="text-base-content/30">//</span>
          {#if hasSearch}
            no flows matching "{searchQuery}"
          {:else}
            no flows
          {/if}
        </div>
      {:else}
        <table class="table table-xs w-full font-mono">
          <thead>
            <tr>
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Pri</th
              >
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Match / Actions</th
              >
            </tr>
          </thead>
          <tbody>
            {#each filteredFlows as flow (flow.uuid)}
              <tr
                class="cursor-pointer border-base-300/60 align-top hover:bg-base-300/40"
                onclick={() => toggleFlow(flow.uuid)}
              >
                <td class="align-top">
                  <Badge text={String(flow.priority)} variant="ghost" />
                </td>
                <td class="text-xs">
                  <!-- Flow summary row -->
                  <div
                    class="break-all font-mono leading-relaxed text-base-content/80"
                  >
                    {flow.match || '1 (any)'}
                  </div>
                  {#if !expandedFlows.has(flow.uuid)}
                    <div class="mt-1 flex flex-wrap gap-1">
                      {#each actionBadges(flow.actions) as badge, i (i)}
                        <Badge text={badge.label} variant={badge.variant} />
                      {/each}
                    </div>
                  {/if}

                  <!-- Expanded details -->
                  {#if expandedFlows.has(flow.uuid)}
                    <div
                      class="mt-2 space-y-2 rounded border border-base-300 bg-base-200 p-3"
                    >
                      <div>
                        <div
                          class="mb-0.5 font-mono text-2xs uppercase tracking-wider text-base-content/55"
                        >
                          Match
                        </div>
                        <div class="break-all font-mono leading-relaxed">
                          {flow.match || '1 (any)'}
                        </div>
                      </div>
                      <div>
                        <div
                          class="mb-0.5 flex items-center gap-2 font-mono text-2xs uppercase tracking-wider text-base-content/55"
                        >
                          Actions
                          {#each actionBadges(flow.actions) as badge, i (i)}
                            <Badge text={badge.label} variant={badge.variant} />
                          {/each}
                        </div>
                        <div class="break-all font-mono leading-relaxed">
                          {flow.actions}
                        </div>
                      </div>
                      {#if flow.external_ids && Object.keys(flow.external_ids).length > 0}
                        <div>
                          <div
                            class="mb-0.5 font-mono text-2xs uppercase tracking-wider text-base-content/55"
                          >
                            External IDs
                          </div>
                          <div class="flex flex-wrap gap-1">
                            {#each Object.entries(flow.external_ids) as [key, value] (key)}
                              {#if key === 'source' && /^[0-9a-f-]{36}$/i.test(value)}
                                <a
                                  href={link(`/nb/acls/${value}`)}
                                  class="badge badge-primary badge-outline badge-sm gap-1"
                                  onclick={(e) => e.stopPropagation()}
                                >
                                  {key}: {value.slice(0, 8)}...
                                </a>
                              {:else}
                                <Badge text={`${key}: ${value}`} variant="ghost" outline />
                              {/if}
                            {/each}
                          </div>
                        </div>
                      {/if}
                      <div
                        class="flex gap-4 border-t border-base-300 pt-1 font-mono text-2xs text-base-content/40"
                      >
                        <span
                          >Priority: <span class="text-base-content/70"
                            >{flow.priority}</span
                          ></span
                        >
                        <span
                          >UUID: <span class="text-base-content/70"
                            >{flow.uuid}</span
                          ></span
                        >
                      </div>
                    </div>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {/if}
</div>
