<script lang="ts">
  import { untrack } from 'svelte';
  import type { ImpactNode } from '../../lib/writeApi';
  import Badge from '../ui/Badge.svelte';
  import ImpactTree from './ImpactTree.svelte';

  let { node, depth = 0 }: { node: ImpactNode; depth?: number } = $props();

  let expanded = $state(untrack(() => depth < 1));

  function refBadge(rt: string): { text: string; variant: string } {
    switch (rt) {
      case 'strong':
        return { text: 'cascade', variant: 'error' };
      case 'weak':
        return { text: 'weak ref', variant: 'warning' };
      case 'reverse':
        return { text: 'references this', variant: 'neutral' };
      case 'correlation':
        return { text: 'SB correlation', variant: 'info' };
      default:
        return { text: rt, variant: 'neutral' };
    }
  }

  let childCount = $derived(node.children?.length ?? 0);
  let badge = $derived(refBadge(node.ref_type));
</script>

<div class={depth > 0 ? 'pl-4' : ''}>
  <div class="flex items-center gap-2 py-0.5">
    {#if childCount > 0}
      <button
        class="btn btn-ghost btn-xs px-0.5"
        onclick={() => (expanded = !expanded)}
        aria-label={expanded ? 'Collapse' : 'Expand'}
      >
        <span class="text-xs">{expanded ? '\u25BE' : '\u25B8'}</span>
      </button>
    {:else}
      <span class="inline-block w-5"></span>
    {/if}

    <span class="font-mono text-xs">{node.table}</span>

    {#if node.name}
      <span class="text-xs font-semibold">{node.name}</span>
    {/if}

    <span class="font-mono text-xs text-base-content/40">
      {node.uuid.slice(0, 8)}
    </span>

    {#if node.ref_type !== 'root'}
      <Badge text={badge.text} variant={badge.variant} />
    {/if}

    {#if childCount > 0 && !expanded}
      <span class="text-xs text-base-content/50">({childCount})</span>
    {/if}
  </div>

  {#if expanded && node.children}
    <div class="ml-4 border-l border-base-300 pl-1">
      {#each node.children as child (child.uuid)}
        <ImpactTree node={child} depth={depth + 1} />
      {/each}
    </div>
  {/if}
</div>
