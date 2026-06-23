<script lang="ts">
  import type { Snippet } from 'svelte';
  import LoadingSpinner from './LoadingSpinner.svelte';
  import ErrorAlert from './ErrorAlert.svelte';
  import EmptyState from './EmptyState.svelte';

  // The unified async boundary. Replaces the loading/error/empty if-else
  // ladder that was copy-pasted across ~25 routes.
  let {
    loading = false,
    error = '',
    empty = false,
    loadingLabel,
    emptyMessage = 'no data',
    emptyHint,
    emptyAction,
    children,
  }: {
    loading?: boolean;
    error?: string;
    empty?: boolean;
    loadingLabel?: string;
    emptyMessage?: string;
    emptyHint?: string;
    emptyAction?: Snippet;
    children: Snippet;
  } = $props();
</script>

{#if loading}
  <LoadingSpinner label={loadingLabel} />
{:else if error}
  <ErrorAlert message={error} />
{:else if empty}
  <EmptyState message={emptyMessage} hint={emptyHint} action={emptyAction} />
{:else}
  {@render children()}
{/if}
