<script lang="ts">
  import { link } from '../../lib/router';
  import Badge from '../ui/Badge.svelte';
  import EnrichmentBadge from './EnrichmentBadge.svelte';

  let {
    title,
    uuid,
    type = '',
    breadcrumbs = [],
    enrichment,
    rawHref,
  }: {
    title: string;
    uuid: string;
    type?: string;
    breadcrumbs?: { label: string; href?: string }[];
    enrichment?: Record<string, unknown> | null;
    rawHref?: string;
  } = $props();
</script>

<div class="mb-4">
  {#if breadcrumbs.length > 0}
    <div class="breadcrumbs mb-1 text-sm">
      <ul>
        {#each breadcrumbs as crumb (crumb.label)}
          <li>
            {#if crumb.href}
              <a href={link(crumb.href)} class="link-hover link"
                >{crumb.label}</a
              >
            {:else}
              {crumb.label}
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  <div class="flex flex-wrap items-center gap-3">
    <h1 class="text-xl font-bold">{title}</h1>
    {#if type}
      <Badge text={type} variant="primary" />
    {/if}
    {#if enrichment}
      <EnrichmentBadge data={enrichment} />
    {/if}
    {#if rawHref}
      <a href={link(rawHref)} class="btn btn-ghost btn-xs">Raw</a>
    {/if}
  </div>
  <p class="mt-1 font-mono text-xs text-base-content/50">{uuid}</p>
</div>
