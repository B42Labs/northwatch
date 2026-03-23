<script lang="ts">
  import { queryEvents, type EventRecord } from '../../lib/api';
  import LoadingSpinner from '../ui/LoadingSpinner.svelte';
  import ErrorAlert from '../ui/ErrorAlert.svelte';

  let events: EventRecord[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let filterDb = $state('');
  let filterTable = $state('');
  let filterType = $state('');
  let timeRange = $state('1h');
  let expandedId: number | null = $state(null);

  function badgeClass(type: string): string {
    switch (type) {
      case 'insert':
        return 'badge-success';
      case 'delete':
        return 'badge-error';
      case 'update':
        return 'badge-warning';
      default:
        return 'badge-ghost';
    }
  }

  function sinceValue(): string | undefined {
    if (timeRange === 'all') return undefined;
    const ms: Record<string, number> = {
      '5m': 5 * 60 * 1000,
      '15m': 15 * 60 * 1000,
      '1h': 60 * 60 * 1000,
      '6h': 6 * 60 * 60 * 1000,
      '24h': 24 * 60 * 60 * 1000,
    };
    const offset = ms[timeRange];
    if (!offset) return undefined;
    return new Date(Date.now() - offset).toISOString();
  }

  async function loadEvents() {
    loading = true;
    error = '';
    try {
      events = await queryEvents({
        since: sinceValue(),
        database: filterDb || undefined,
        table: filterTable || undefined,
        type: filterType || undefined,
        limit: 500,
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load events';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void filterDb;
    void filterTable;
    void filterType;
    void timeRange;
    loadEvents();
  });
</script>

<div>
  <div class="mb-3 flex flex-wrap items-center gap-2">
    <select bind:value={filterDb} class="select select-bordered select-sm">
      <option value="">All databases</option>
      <option value="nb">nb</option>
      <option value="sb">sb</option>
    </select>

    <input
      type="text"
      bind:value={filterTable}
      placeholder="Table name..."
      class="input input-sm input-bordered w-40"
    />

    <select bind:value={filterType} class="select select-bordered select-sm">
      <option value="">All types</option>
      <option value="insert">insert</option>
      <option value="update">update</option>
      <option value="delete">delete</option>
    </select>

    <div class="join">
      {#each ['5m', '15m', '1h', '6h', '24h', 'all'] as range (range)}
        <button
          class="btn join-item btn-xs {timeRange === range ? 'btn-active' : ''}"
          onclick={() => (timeRange = range)}
        >
          {range}
        </button>
      {/each}
    </div>

    <button class="btn btn-ghost btn-xs" onclick={loadEvents}>Refresh</button>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if loading && events.length === 0}
    <LoadingSpinner />
  {:else if events.length === 0}
    <div class="py-8 text-center text-sm text-base-content/40">
      No events in the selected time range.
    </div>
  {:else}
    <div class="mb-2 text-xs text-base-content/50">
      {events.length} events
    </div>
    <div class="max-h-[600px] overflow-auto">
      <table class="table table-xs">
        <thead class="sticky top-0 bg-base-100">
          <tr>
            <th>Time</th>
            <th>Type</th>
            <th>Database</th>
            <th>Table</th>
            <th>UUID</th>
          </tr>
        </thead>
        <tbody>
          {#each events as evt (evt.id)}
            <tr
              class="hover cursor-pointer"
              onclick={() =>
                (expandedId = expandedId === evt.id ? null : evt.id)}
            >
              <td class="whitespace-nowrap text-xs text-base-content/60">
                {new Date(evt.timestamp).toLocaleTimeString()}
              </td>
              <td>
                <span class="badge badge-xs {badgeClass(evt.type)}"
                  >{evt.type}</span
                >
              </td>
              <td>{evt.database}</td>
              <td>{evt.table}</td>
              <td class="font-mono text-xs">{evt.uuid.slice(0, 12)}</td>
            </tr>
            {#if expandedId === evt.id}
              <tr>
                <td colspan="5" class="bg-base-200 p-2">
                  {#if evt.old_row}
                    <div class="mb-2">
                      <div
                        class="mb-1 text-xs font-semibold text-base-content/50"
                      >
                        Old Row
                      </div>
                      <pre
                        class="max-h-32 overflow-auto rounded bg-base-100 p-2 text-xs">{JSON.stringify(
                          evt.old_row,
                          null,
                          2,
                        )}</pre>
                    </div>
                  {/if}
                  {#if evt.row}
                    <div>
                      <div
                        class="mb-1 text-xs font-semibold text-base-content/50"
                      >
                        Row
                      </div>
                      <pre
                        class="max-h-32 overflow-auto rounded bg-base-100 p-2 text-xs">{JSON.stringify(
                          evt.row,
                          null,
                          2,
                        )}</pre>
                    </div>
                  {/if}
                  {#if !evt.row && !evt.old_row}
                    <div class="text-xs text-base-content/40">
                      No row data available.
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
