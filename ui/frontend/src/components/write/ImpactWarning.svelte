<script lang="ts">
  import type { ImpactEntry } from '../../lib/writeApi';
  import Badge from '../ui/Badge.svelte';
  import ImpactTree from './ImpactTree.svelte';

  let { entries }: { entries: ImpactEntry[] } = $props();

  let showTree = $state(false);

  let totalAffected = $derived(
    entries.reduce((sum, e) => sum + e.result.summary.total_affected, 0),
  );

  let tableBreakdown = $derived.by(() => {
    const merged: Record<string, number> = {};
    for (const e of entries) {
      for (const [table, count] of Object.entries(e.result.summary.by_table)) {
        merged[table] = (merged[table] ?? 0) + count;
      }
    }
    return Object.entries(merged).sort((a, b) => b[1] - a[1]);
  });
</script>

<div role="alert" class="alert alert-warning shadow-sm">
  <svg
    xmlns="http://www.w3.org/2000/svg"
    class="h-5 w-5 shrink-0"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      stroke-linecap="round"
      stroke-linejoin="round"
      stroke-width="2"
      d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
    />
  </svg>

  <div class="flex flex-col gap-1">
    <span class="text-sm font-semibold">
      Delete impacts {totalAffected} dependent object{totalAffected !== 1
        ? 's'
        : ''}
    </span>

    <div class="flex flex-wrap gap-1.5">
      {#each tableBreakdown as [table, count]}
        <Badge text="{count} {table}" variant="ghost" />
      {/each}
      {#if entries.some((e) => e.result.summary.truncated)}
        <Badge text="truncated" variant="warning" />
      {/if}
    </div>

    <button
      class="btn btn-ghost btn-xs mt-1 w-fit"
      onclick={() => (showTree = !showTree)}
    >
      {showTree ? 'Hide' : 'Show'} dependency tree
    </button>

    {#if showTree}
      <div class="mt-2 rounded-lg bg-base-100 p-3">
        {#each entries as entry (entry.operation_index)}
          <ImpactTree node={entry.result.root} />
        {/each}
      </div>
    {/if}
  </div>
</div>
