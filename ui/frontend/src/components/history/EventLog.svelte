<script lang="ts">
  import { queryEvents, type EventRecord } from '../../lib/api';
  import { actionVariant, actionGlyph } from '../../lib/status';
  import DataState from '../ui/DataState.svelte';
  import FilterInput from '../ui/FilterInput.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Badge from '../ui/Badge.svelte';

  let events: EventRecord[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let filterDb = $state('');
  let filterTable = $state('');
  let filterType = $state('');
  let timeRange = $state('1h');
  let expandedId: number | null = $state(null);

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

    <button class="btn btn-ghost btn-xs border-base-300" onclick={loadEvents}
      >Refresh</button
    >
  </div>

  <DataState
    {loading}
    {error}
    empty={events.length === 0}
    emptyMessage="no events in the selected time range"
  >
    <div class="mb-2 font-mono text-xs text-base-content/55">
      {events.length} events
    </div>
    <div class="max-h-[600px] overflow-auto rounded border border-base-300">
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
              class="cursor-pointer border-base-300/60 hover:bg-base-300/40"
              onclick={() =>
                (expandedId = expandedId === evt.id ? null : evt.id)}
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
            {#if expandedId === evt.id}
              <tr>
                <td colspan="5" class="bg-base-200/40 p-2">
                  {#if evt.old_row}
                    <div class="mb-2">
                      <div
                        class="mb-1 font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
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
                        class="mb-1 font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
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
                    <div class="font-mono text-sm text-base-content/40">
                      <span class="text-base-content/30">//</span> no row data available
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  </DataState>
</div>
