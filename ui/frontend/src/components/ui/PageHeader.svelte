<script lang="ts">
  import type { Snippet } from 'svelte';
  import { link } from '../../lib/router';

  // The single page-header for the whole app. Replaces ~25 hand-rolled
  // headers (and EntityHeader, which now wraps this).
  let {
    title,
    eyebrow,
    description,
    breadcrumbs = [],
    mono = false,
    actions,
    meta,
  }: {
    title: string;
    eyebrow?: string;
    description?: string;
    breadcrumbs?: { label: string; href?: string }[];
    /** Render the title in monospace (entity ids, raw views). */
    mono?: boolean;
    actions?: Snippet;
    meta?: Snippet;
  } = $props();
</script>

<header
  class="mb-5 flex flex-wrap items-end justify-between gap-x-4 gap-y-2 border-b border-base-300 pb-3"
>
  <div class="min-w-0">
    {#if breadcrumbs.length > 0}
      <nav
        class="mb-1 flex flex-wrap items-center gap-1 font-mono text-2xs uppercase tracking-wider text-base-content/45"
      >
        {#each breadcrumbs as crumb, i (crumb.label)}
          {#if i > 0}<span class="text-base-content/25">/</span>{/if}
          {#if crumb.href}
            <a
              href={link(crumb.href)}
              class="transition-colors hover:text-primary">{crumb.label}</a
            >
          {:else}
            <span>{crumb.label}</span>
          {/if}
        {/each}
      </nav>
    {:else if eyebrow}
      <div
        class="mb-1 font-mono text-2xs uppercase tracking-widest text-base-content/45"
      >
        {eyebrow}
      </div>
    {/if}

    <h1
      class="flex items-baseline gap-2 text-xl font-semibold leading-tight text-base-content {mono
        ? 'break-all font-mono'
        : ''}"
    >
      <span class="nw-glow select-none text-primary" aria-hidden="true"
        >&gt;</span
      >
      <span class="min-w-0">{title}</span>
    </h1>

    {#if meta}
      <div class="mt-1.5 flex flex-wrap items-center gap-2">
        {@render meta()}
      </div>
    {/if}

    {#if description}
      <p class="font-prose mt-1.5 max-w-2xl text-sm text-base-content/55">
        {description}
      </p>
    {/if}
  </div>

  {#if actions}
    <div class="flex flex-shrink-0 flex-wrap items-center gap-2">
      {@render actions()}
    </div>
  {/if}
</header>
