<script lang="ts">
  import type { Snippet } from 'svelte';

  // Replaces the repeated `form-control` + `label`/`label-text` pairs (~20).
  let {
    label,
    forId,
    hint,
    children,
  }: {
    label: string;
    forId?: string;
    hint?: string;
    children: Snippet;
  } = $props();
</script>

<div class="flex flex-col gap-1">
  <label
    class="font-mono text-2xs uppercase tracking-wider text-base-content/60"
    for={forId}>{label}</label
  >
  {@render children()}
  {#if hint}
    <span class="font-prose text-2xs text-base-content/40">{hint}</span>
  {/if}
</div>
