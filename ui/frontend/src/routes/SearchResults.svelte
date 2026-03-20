<script lang="ts">
  import { search, type SearchResponse } from '../lib/api';
  import { push, link } from '../lib/router';
  import { tableSlugFromOvsdbName } from '../lib/tables';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import Badge from '../components/ui/Badge.svelte';

  let { query }: { query: string } = $props();

  let result: SearchResponse | null = $state(null);
  let loading = $state(false);
  let error = $state('');

  async function doSearch(q: string) {
    if (!q) return;
    loading = true;
    error = '';
    try {
      result = await search(q);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Search failed';
      result = null;
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    doSearch(query);
  });

  let totalMatches = $derived(
    result?.results?.reduce((sum, g) => sum + (g.matches?.length ?? 0), 0) ?? 0,
  );

  let groupLimits: Record<string, number> = $state({});

  function groupLimit(key: string): number {
    return groupLimits[key] ?? 20;
  }

  function showMoreGroup(key: string) {
    groupLimits[key] = (groupLimits[key] ?? 20) + 20;
  }

  function dbLabel(db: string): string {
    return db === 'nb' || db === 'OVN_Northbound' ? 'NB' : 'SB';
  }

  function tableSlug(table: string): string {
    return tableSlugFromOvsdbName(table);
  }

  function dbKey(db: string): string {
    return db === 'nb' || db === 'OVN_Northbound' ? 'nb' : 'sb';
  }
</script>

<div>
  <h1 class="mb-1 text-xl font-bold">Search Results</h1>
  {#if query}
    <p class="mb-4 text-sm text-base-content/60">
      Query: <span class="font-mono font-semibold">{query}</span>
      {#if result?.query_type}
        <Badge text={result.query_type} variant="info" />
      {/if}
      {#if !loading}
        - {totalMatches} match{totalMatches !== 1 ? 'es' : ''}
      {/if}
    </p>
  {/if}

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else if result && totalMatches === 0}
    <div class="py-8 text-center text-base-content/50">No results found</div>
  {:else if result}
    <div class="flex flex-col gap-4">
      {#each result.results as group}
        {#if group.matches && group.matches.length > 0}
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body p-4">
              <h2 class="card-title text-sm">
                <Badge text={dbLabel(group.database)} variant="neutral" />
                {group.table}
                <span class="text-xs font-normal text-base-content/50">
                  ({group.matches.length})
                </span>
              </h2>
              <div class="overflow-x-auto">
                <table class="table table-zebra table-xs">
                  <tbody>
                    {#each group.matches.slice(0, groupLimit(`${group.database}:${group.table}`)) as match}
                      {@const uuid = match._uuid as string}
                      <tr
                        class="hover cursor-pointer"
                        onclick={() =>
                          push(
                            `/${dbKey(group.database)}/${tableSlug(group.table)}/${uuid}`,
                          )}
                      >
                        <td class="font-mono text-xs">
                          <a
                            href={link(
                              `/${dbKey(group.database)}/${tableSlug(group.table)}/${uuid}`,
                            )}
                            class="link link-primary"
                          >
                            {uuid ? uuid.slice(0, 8) : '-'}
                          </a>
                        </td>
                        <td class="text-xs">
                          {#if match.name}
                            {match.name}
                          {:else if match.logical_port}
                            {match.logical_port}
                          {:else if match.hostname}
                            {match.hostname}
                          {:else if match.match}
                            <span class="font-mono"
                              >{String(match.match).slice(0, 80)}</span
                            >
                          {:else}
                            <span class="text-base-content/40">-</span>
                          {/if}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
                {#if group.matches.length > groupLimit(`${group.database}:${group.table}`)}
                  <button
                    class="btn btn-ghost btn-sm mt-2"
                    onclick={() =>
                      showMoreGroup(`${group.database}:${group.table}`)}
                  >
                    Show more (showing {groupLimit(
                      `${group.database}:${group.table}`,
                    )} of {group.matches.length})
                  </button>
                {/if}
              </div>
            </div>
          </div>
        {/if}
      {/each}
    </div>
  {/if}
</div>
