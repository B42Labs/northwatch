<script lang="ts">
  import type { Snippet } from 'svelte';

  // Resolves the long-standing inconsistency where some pages used
  // `mx-auto max-w-4xl` and others ran full-bleed. Width is now a prop.
  let {
    width = 'full',
    children,
  }: {
    width?: 'full' | 'wide' | 'prose';
    children: Snippet;
  } = $props();

  const widths = {
    full: 'w-full',
    wide: 'mx-auto w-full max-w-7xl',
    prose: 'mx-auto w-full max-w-4xl',
  };
</script>

<div class={widths[width]}>
  {@render children()}
</div>
