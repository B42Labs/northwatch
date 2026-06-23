<script lang="ts">
  import type { Snippet } from 'svelte';
  import { accentBorderClass, type Variant } from '../../lib/status';

  // The single panel primitive. Replaces 30+ ad-hoc
  // `card bg-base-100 shadow-sm` + `card-body p-4` blocks.
  let {
    title,
    subtitle,
    accent,
    padding = 'md',
    actions,
    children,
    class: extra = '',
  }: {
    title?: string;
    subtitle?: string;
    /** Left-edge accent stripe + header tint. */
    accent?: Variant;
    padding?: 'none' | 'sm' | 'md';
    actions?: Snippet;
    children: Snippet;
    class?: string;
  } = $props();

  const pad = { none: '', sm: 'p-2.5', md: 'p-4' };
  let hasHeader = $derived(Boolean(title || subtitle || actions));
</script>

<section
  class="overflow-hidden rounded border border-base-300 bg-base-100 {accent
    ? `border-l-2 ${accentBorderClass[accent]}`
    : ''} {extra}"
>
  {#if hasHeader}
    <header
      class="flex items-center justify-between gap-2 border-b border-base-300 bg-base-200/40 px-3 py-2"
    >
      <div class="flex min-w-0 items-baseline gap-2">
        {#if title}
          <h2
            class="truncate font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
          >
            {title}
          </h2>
        {/if}
        {#if subtitle}
          <span class="truncate font-mono text-2xs text-base-content/40"
            >{subtitle}</span
          >
        {/if}
      </div>
      {#if actions}
        <div class="flex flex-shrink-0 items-center gap-1">{@render actions()}</div>
      {/if}
    </header>
  {/if}
  <div class={pad[padding]}>
    {@render children()}
  </div>
</section>
