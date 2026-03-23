<script lang="ts">
  import { queryEvents, type EventRecord } from '../lib/api';
  import { subscribeToTables } from '../lib/eventStore';
  import type { WsEvent } from '../lib/websocket';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import EventDetailPanel from '../components/history/EventDetailPanel.svelte';

  let events: EventRecord[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let filterDb = $state('');
  let filterTable = $state('');
  let filterType = $state('');
  let timeRange = $state('1h');
  let selectedEvent: EventRecord | null = $state(null);

  // Live view
  let liveUpdates = $state(false);
  let liveEventId = 0;

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

  function matchesFilters(evt: {
    database: string;
    table: string;
    type: string;
  }): boolean {
    if (filterDb && evt.database !== filterDb) return false;
    if (
      filterTable &&
      !evt.table.toLowerCase().includes(filterTable.toLowerCase())
    )
      return false;
    if (filterType && evt.type !== filterType) return false;
    return true;
  }

  // Live updates via WebSocket
  $effect(() => {
    if (!liveUpdates) return;

    const unsubscribe = subscribeToTables('*', ['*'], (wsEvent: WsEvent) => {
      const liveRecord: EventRecord = {
        id: --liveEventId,
        timestamp: wsEvent.timestamp ?? new Date().toISOString(),
        type: wsEvent.type,
        database: wsEvent.database,
        table: wsEvent.table,
        uuid: wsEvent.uuid,
        row: wsEvent.row as Record<string, unknown> | undefined,
        old_row: wsEvent.old_row as Record<string, unknown> | undefined,
      };

      if (matchesFilters(liveRecord)) {
        events = [liveRecord, ...events].slice(0, 1000);
      }
    });

    return () => {
      unsubscribe();
    };
  });

  // Reload on filter change (only when not in live mode for initial load)
  $effect(() => {
    void filterDb;
    void filterTable;
    void filterType;
    void timeRange;
    loadEvents();
  });
</script>

<div class="flex h-full flex-col">
  <div class="mb-4">
    <h1 class="text-xl font-bold">Events</h1>
    <p class="text-sm text-base-content/60">
      Real-time and historical OVN database change events
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {/if}

  <!-- Toolbar -->
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

    <label
      class="ml-2 flex cursor-pointer select-none items-center gap-2 text-sm"
    >
      <input
        type="checkbox"
        bind:checked={liveUpdates}
        class="checkbox checkbox-sm"
      />
      Live updates
      {#if liveUpdates}
        <span class="badge badge-success badge-xs animate-pulse">live</span>
      {/if}
    </label>

    <button class="btn btn-ghost btn-xs ml-auto" onclick={loadEvents}
      >Refresh</button
    >
  </div>

  <!-- Event table -->
  {#if loading && events.length === 0}
    <LoadingSpinner />
  {:else if events.length === 0}
    <div class="py-8 text-center text-sm text-base-content/40">
      No events in the selected time range.
    </div>
  {:else}
    <div class="mb-2 text-xs text-base-content/50">
      {events.length} events
    </div>
    <div class="flex-1 overflow-auto">
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
              class="hover cursor-pointer {selectedEvent?.id === evt.id
                ? 'bg-primary/10'
                : ''} {evt.id < 0 ? 'animate-fade-in' : ''}"
              onclick={() =>
                (selectedEvent = selectedEvent?.id === evt.id ? null : evt)}
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
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Detail Panel -->
{#if selectedEvent}
  <EventDetailPanel
    event={selectedEvent}
    onClose={() => (selectedEvent = null)}
  />
{/if}

<style>
  @keyframes fade-in {
    from {
      background-color: oklch(var(--s) / 0.2);
    }
    to {
      background-color: transparent;
    }
  }

  .animate-fade-in {
    animation: fade-in 1.5s ease-out;
  }
</style>
