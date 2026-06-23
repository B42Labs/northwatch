<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    message = 'no data',
    hint,
    action,
  }: {
    message?: string;
    hint?: string;
    action?: Snippet;
  } = $props();
</script>

<div
  class="flex flex-col items-center justify-center gap-2 rounded border border-dashed border-base-300 px-6 py-12 text-center"
>
  <span class="font-mono text-sm text-base-content/45"
    ><span class="text-base-content/30">//</span> {message}</span
  >
  {#if hint}
    <span class="max-w-sm font-prose text-xs text-base-content/35">{hint}</span>
  {/if}
  {#if action}
    <div class="mt-1">{@render action()}</div>
  {/if}
</div>
