<script lang="ts">
  import { link } from '../../lib/router';
  import Badge from '../ui/Badge.svelte';
  import EnrichmentBadge from './EnrichmentBadge.svelte';
  import PageHeader from '../ui/PageHeader.svelte';

  // Thin wrapper over the shared PageHeader for entity profile pages.
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

<PageHeader {title} {breadcrumbs}>
  {#snippet meta()}
    {#if type}
      <Badge text={type} variant="primary" />
    {/if}
    {#if enrichment}
      <EnrichmentBadge data={enrichment} />
    {/if}
    <span class="font-mono text-xs text-base-content/50 break-all">{uuid}</span>
  {/snippet}
  {#snippet actions()}
    {#if rawHref}
      <a href={link(rawHref)} class="btn btn-ghost btn-xs border-base-300"
        >Raw</a
      >
    {/if}
  {/snippet}
</PageHeader>
