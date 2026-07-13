<script lang="ts">
  import { search, type SearchResponse } from '../lib/api';
  import { push, link } from '../lib/router';
  import { tableSlugFromOvsdbName } from '../lib/tables';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Card from '../components/ui/Card.svelte';
  import Badge from '../components/ui/Badge.svelte';

  let { query }: { query: string } = $props();

  let result = $state<SearchResponse | null>(null);
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

<PageHeader eyebrow="Search" title="Search results">
  {#snippet meta()}
    {#if query}
      <span class="font-mono text-xs text-base-content/55">
        query <span class="text-base-content/80">{query}</span>
      </span>
      {#if result?.query_type}
        <Badge text={result.query_type} variant="info" />
      {/if}
      {#if !loading}
        <span class="font-mono text-xs text-base-content/55"
          >{totalMatches} match{totalMatches !== 1 ? 'es' : ''}</span
        >
      {/if}
    {/if}
  {/snippet}
</PageHeader>

<DataState
  {loading}
  {error}
  empty={!!result && totalMatches === 0}
  emptyMessage="no results"
>
  {#if result}
    <div class="flex flex-col gap-4">
      {#if result.truncated}
        <div
          class="rounded border border-warning/40 bg-warning/10 px-3 py-2 font-mono text-xs text-base-content/80"
        >
          results truncated — showing the first {totalMatches} matches; refine the
          query to narrow them down
        </div>
      {/if}
      {#each result.results as group (`${group.database}:${group.table}`)}
        {#if group.matches && group.matches.length > 0}
          <Card padding="none">
            <div class="border-b border-base-300 bg-base-200/40 px-3 py-2">
              <h2
                class="flex items-baseline gap-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
              >
                <Badge text={dbLabel(group.database)} variant="neutral" />
                <span class="truncate">{group.table}</span>
                <span class="font-mono text-2xs text-base-content/40"
                  >{group.matches.length}</span
                >
              </h2>
            </div>
            <div class="overflow-x-auto">
              <table class="table table-xs font-mono">
                <tbody>
                  {#each group.matches.slice(0, groupLimit(`${group.database}:${group.table}`)) as match (match._uuid)}
                    {@const uuid = match._uuid as string}
                    <tr
                      class="cursor-pointer border-base-300/60 hover:bg-base-300/40"
                      onclick={() =>
                        push(
                          `/${dbKey(group.database)}/${tableSlug(group.table)}/${uuid}`,
                        )}
                    >
                      <td class="text-xs">
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
                  class="btn m-2 border-base-300 btn-ghost btn-sm"
                  onclick={() =>
                    showMoreGroup(`${group.database}:${group.table}`)}
                >
                  Show more (showing {groupLimit(
                    `${group.database}:${group.table}`,
                  )} of {group.matches.length})
                </button>
              {/if}
            </div>
          </Card>
        {/if}
      {/each}
    </div>
  {/if}
</DataState>
