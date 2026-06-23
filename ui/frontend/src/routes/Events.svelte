<script lang="ts">
  import { queryEvents, type EventRecord } from '../lib/api';
  import { subscribeToTables } from '../lib/eventStore';
  import type { WsEvent } from '../lib/websocket';
  import { actionVariant, actionGlyph } from '../lib/status';
  import PageContainer from '../components/ui/PageContainer.svelte';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import Badge from '../components/ui/Badge.svelte';
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

  const timeRangeOptions = [
    { value: '5m', label: '5m' },
    { value: '15m', label: '15m' },
    { value: '1h', label: '1h' },
    { value: '6h', label: '6h' },
    { value: '24h', label: '24h' },
    { value: 'all', label: 'all' },
  ];

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

<PageContainer width="wide">
  <div class="flex h-full flex-col">
    <PageHeader
      eyebrow="Events"
      title="Events"
      description="Real-time and historical OVN database change events"
    />

    <!-- Toolbar -->
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <select
        bind:value={filterDb}
        class="select select-bordered select-sm bg-base-200/60 font-mono"
      >
        <option value="">All databases</option>
        <option value="nb">nb</option>
        <option value="sb">sb</option>
      </select>

      <FilterInput
        bind:value={filterTable}
        placeholder="table name…"
        width="w-40"
      />

      <select
        bind:value={filterType}
        class="select select-bordered select-sm bg-base-200/60 font-mono"
      >
        <option value="">All types</option>
        <option value="insert">insert</option>
        <option value="update">update</option>
        <option value="delete">delete</option>
      </select>

      <SegmentedControl
        options={timeRangeOptions}
        bind:value={timeRange}
        size="xs"
      />

      <label
        class="ml-2 flex cursor-pointer select-none items-center gap-2 font-mono text-sm"
      >
        <input
          type="checkbox"
          bind:checked={liveUpdates}
          class="checkbox checkbox-sm"
        />
        Live updates
        {#if liveUpdates}
          <Badge text="live" variant="success" glyph="●" />
        {/if}
      </label>

      <button
        class="btn btn-ghost btn-xs ml-auto border-base-300"
        onclick={loadEvents}>Refresh</button
      >
    </div>

    <!-- Event table -->
    <DataState
      loading={loading && events.length === 0}
      {error}
      empty={events.length === 0}
      emptyMessage="no events in the selected time range"
    >
      <div class="mb-2 font-mono text-xs text-base-content/55">
        {events.length} events
      </div>
      <div class="flex-1 overflow-auto rounded border border-base-300">
        <table class="table table-pin-rows table-xs font-mono">
          <thead>
            <tr>
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Time</th
              >
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Type</th
              >
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Database</th
              >
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Table</th
              >
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >UUID</th
              >
            </tr>
          </thead>
          <tbody>
            {#each events as evt (evt.id)}
              <tr
                class="cursor-pointer border-base-300/60 hover:bg-base-300/40 {selectedEvent?.id ===
                evt.id
                  ? 'bg-primary/10'
                  : ''} {evt.id < 0 ? 'animate-fade-in' : ''}"
                onclick={() =>
                  (selectedEvent = selectedEvent?.id === evt.id ? null : evt)}
              >
                <td class="whitespace-nowrap text-xs text-base-content/60">
                  {new Date(evt.timestamp).toLocaleTimeString()}
                </td>
                <td>
                  <Badge
                    text={evt.type}
                    variant={actionVariant(evt.type)}
                    glyph={actionGlyph(evt.type)}
                  />
                </td>
                <td>{evt.database}</td>
                <td>{evt.table}</td>
                <td class="text-xs">{evt.uuid.slice(0, 12)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </DataState>
  </div>
</PageContainer>

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
