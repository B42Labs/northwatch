<script lang="ts">
  import { link } from '../../lib/router';
  import Badge from '../ui/Badge.svelte';
  import EnrichmentBadge from './EnrichmentBadge.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import CopyViewButtons from '../ui/CopyViewButtons.svelte';

  // Thin wrapper over the shared PageHeader for entity profile pages.
  let {
    title,
    uuid,
    type = '',
    breadcrumbs = [],
    enrichment,
    rawHref,
    view,
  }: {
    title: string;
    uuid: string;
    type?: string;
    breadcrumbs?: { label: string; href?: string }[];
    enrichment?: Record<string, unknown> | null;
    rawHref?: string;
    /** Returns the correlated record to copy. Omitted, no copy action shows. */
    view?: () => unknown;
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
    <span class="font-mono text-xs break-all text-base-content/50">{uuid}</span>
  {/snippet}
  {#snippet actions()}
    {#if view}
      <CopyViewButtons {title} {view} />
    {/if}
    {#if rawHref}
      <a href={link(rawHref)} class="btn border-base-300 btn-ghost btn-xs"
        >Raw</a
      >
    {/if}
  {/snippet}
</PageHeader>
