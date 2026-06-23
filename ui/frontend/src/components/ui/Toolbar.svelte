<script lang="ts">
  import type { Snippet } from 'svelte';

  // Consistent spacing/alignment for a filter+actions row. `end` is pushed
  // to the right (counts, toggles, action buttons).
  let {
    children,
    end,
  }: {
    children: Snippet;
    end?: Snippet;
  } = $props();
</script>

<div class="mb-3 flex flex-wrap items-center gap-2">
  {@render children()}
  {#if end}
    <div class="ml-auto flex flex-wrap items-center gap-2">{@render end()}</div>
  {/if}
</div>
