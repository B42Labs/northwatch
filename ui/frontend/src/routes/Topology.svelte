<script lang="ts">
  import {
    getTopology,
    type TopologyNode,
    type TopologyEdge,
    type EventRecord,
  } from '../lib/api';
  import TopologyGraph from '../components/topology/TopologyGraph.svelte';
  import TopologyEventStream, {
    type StreamEvent,
  } from '../components/topology/TopologyEventStream.svelte';
  import EventDetailPanel from '../components/history/EventDetailPanel.svelte';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { subscribeToTables } from '../lib/eventStore';
  import { exportSVG, exportPNG, exportJSON } from '../lib/export';

  let allNodes: TopologyNode[] = $state([]);
  let allEdges: TopologyEdge[] = $state([]);
  let loading = $state(true);
  let error = $state('');
  let showVMs = $state(false);
  let focusNetwork = $state('');
  let focusChassis = $state('');
  let searchQuery = $state('');
  let relayoutKey = $state(0);
  let liveUpdates = $state(false);
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;
  let topologySvgRef: SVGSVGElement | undefined = $state(undefined);

  // Live event stream — captures the WebSocket changes that drive live
  // updates so they can be shown as an add/change/delete feed over the graph.
  const MAX_STREAM_EVENTS = 100;
  let liveEvents: StreamEvent[] = $state([]);
  let streamSeq = 0;
  let selectedEvent: EventRecord | null = $state(null);

  function showEventDetail(e: StreamEvent) {
    selectedEvent = {
      id: e.seq,
      timestamp: new Date(e.ts).toISOString(),
      type: e.type,
      database: e.database,
      table: e.table,
      uuid: e.uuid,
      row: e.row as Record<string, unknown> | undefined,
      old_row: e.old_row as Record<string, unknown> | undefined,
    };
  }

  function handleExportSVG() {
    if (topologySvgRef) exportSVG(topologySvgRef, 'northwatch-topology.svg');
  }
  function handleExportPNG() {
    if (topologySvgRef) exportPNG(topologySvgRef, 'northwatch-topology.png');
  }
  function handleExportJSON() {
    exportJSON({ nodes, edges }, 'northwatch-topology.json');
  }

  // --- Export menu (controlled, keyboard-accessible) ---
  let exportMenuOpen = $state(false);
  let exportMenuEl: HTMLDivElement | undefined = $state();

  function exportItems(): HTMLButtonElement[] {
    return exportMenuEl
      ? [
          ...exportMenuEl.querySelectorAll<HTMLButtonElement>(
            '[role="menuitem"]',
          ),
        ]
      : [];
  }

  function openExportMenu() {
    exportMenuOpen = true;
    requestAnimationFrame(() => exportItems()[0]?.focus());
  }

  function closeExportMenu(refocusTrigger = false) {
    exportMenuOpen = false;
    if (refocusTrigger) {
      exportMenuEl
        ?.querySelector<HTMLButtonElement>('button[aria-haspopup]')
        ?.focus();
    }
  }

  function runExport(fn: () => void) {
    fn();
    closeExportMenu();
  }

  function onExportTriggerKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      openExportMenu();
    } else if (e.key === 'Escape') {
      closeExportMenu();
    }
  }

  function onExportMenuKey(e: KeyboardEvent) {
    const items = exportItems();
    if (items.length === 0) return;
    const idx = items.indexOf(document.activeElement as HTMLButtonElement);
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        items[(idx + 1) % items.length]?.focus();
        break;
      case 'ArrowUp':
        e.preventDefault();
        items[(idx - 1 + items.length) % items.length]?.focus();
        break;
      case 'Home':
        e.preventDefault();
        items[0]?.focus();
        break;
      case 'End':
        e.preventDefault();
        items[items.length - 1]?.focus();
        break;
      case 'Escape':
        e.preventDefault();
        closeExportMenu(true);
        break;
    }
  }

  function onExportFocusOut(e: FocusEvent) {
    const next = e.relatedTarget as Node | null;
    if (!next || (exportMenuEl && !exportMenuEl.contains(next))) {
      exportMenuOpen = false;
    }
  }

  // Dropdown options derived from data
  let networkOptions = $derived(
    allNodes
      .filter((n) => n.type === 'switch')
      .map((n) => ({ id: n.id, label: n.label }))
      .sort((a, b) => a.label.localeCompare(b.label)),
  );
  let chassisOptions = $derived(
    allNodes
      .filter((n) => n.type === 'chassis')
      .map((n) => ({
        id: n.id,
        label: n.label,
        gw: n.metadata?.role === 'gateway',
      }))
      .sort((a, b) => a.label.localeCompare(b.label)),
  );

  // Collect reachable node IDs from a starting node via BFS (max 2 hops)
  function reachableNodes(startId: string, maxHops: number): Set<string> {
    const visited = new SvelteSet<string>();
    let frontier = new SvelteSet([startId]);
    for (let hop = 0; hop <= maxHops && frontier.size > 0; hop++) {
      for (const id of frontier) visited.add(id);
      const next = new SvelteSet<string>();
      for (const id of frontier) {
        for (const e of allEdges) {
          if (e.source === id && !visited.has(e.target)) next.add(e.target);
          if (e.target === id && !visited.has(e.source)) next.add(e.source);
        }
      }
      frontier = next;
    }
    return visited;
  }

  // Filtered data
  let nodes = $derived.by(() => {
    if (!focusNetwork && !focusChassis) return allNodes;
    const focusId = focusNetwork || focusChassis;
    const visible = reachableNodes(focusId, 2);
    return allNodes.filter((n) => visible.has(n.id));
  });

  let edges = $derived.by(() => {
    if (!focusNetwork && !focusChassis) return allEdges;
    const nodeIds = new Set(nodes.map((n) => n.id));
    return allEdges.filter(
      (e) => nodeIds.has(e.source) && nodeIds.has(e.target),
    );
  });

  async function load() {
    try {
      const data = await getTopology({ vms: showVMs });
      allNodes = data.nodes;
      allEdges = data.edges;
      error = '';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load topology';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load();
  });

  $effect(() => {
    if (!liveUpdates) {
      liveEvents = [];
      return;
    }
    const unsubscribe = subscribeToTables(
      '*',
      [
        'Logical_Switch',
        'Logical_Switch_Port',
        'Logical_Router',
        'Logical_Router_Port',
        'Chassis',
        'Port_Binding',
      ],
      (event) => {
        liveEvents = [{ ...event, seq: ++streamSeq }, ...liveEvents].slice(
          0,
          MAX_STREAM_EVENTS,
        );
        if (refetchTimer) clearTimeout(refetchTimer);
        refetchTimer = setTimeout(() => {
          if (!loading) load();
        }, 500);
      },
    );
    return () => {
      unsubscribe();
      if (refetchTimer) clearTimeout(refetchTimer);
    };
  });

  $effect(() => {
    void showVMs;
    load();
  });

  function shortLabel(label: string): string {
    const m = label.match(/^neutron-([a-f0-9]{8})/);
    return m ? m[1] : label;
  }

  // Searchable dropdown state
  let networkSearch = $state('');
  let chassisSearch = $state('');
  let networkDropdownOpen = $state(false);
  let chassisDropdownOpen = $state(false);
  let networkComboEl: HTMLDivElement | undefined = $state();
  let chassisComboEl: HTMLDivElement | undefined = $state();

  let filteredNetworks = $derived(
    networkOptions.filter((o) => {
      if (!networkSearch) return true;
      const q = networkSearch.toLowerCase();
      return (
        o.label.toLowerCase().includes(q) || o.id.toLowerCase().includes(q)
      );
    }),
  );

  let filteredChassis = $derived(
    chassisOptions.filter((o) => {
      if (!chassisSearch) return true;
      const q = chassisSearch.toLowerCase();
      return (
        o.label.toLowerCase().includes(q) || o.id.toLowerCase().includes(q)
      );
    }),
  );

  function selectNetwork(id: string, label: string) {
    focusNetwork = id;
    focusChassis = '';
    networkSearch = shortLabel(label);
    networkDropdownOpen = false;
  }

  function selectChassis(id: string, label: string, gw: boolean) {
    focusChassis = id;
    focusNetwork = '';
    chassisSearch = label + (gw ? ' (GW)' : '');
    chassisDropdownOpen = false;
  }

  function clearNetwork() {
    focusNetwork = '';
    networkSearch = '';
    networkDropdownOpen = false;
  }

  function clearChassis() {
    focusChassis = '';
    chassisSearch = '';
    chassisDropdownOpen = false;
  }

  function clearFilters() {
    focusNetwork = '';
    focusChassis = '';
    searchQuery = '';
    networkSearch = '';
    chassisSearch = '';
    networkDropdownOpen = false;
    chassisDropdownOpen = false;
  }

  // Combobox option elements, for roving keyboard focus into the listbox.
  function comboOptions(el: HTMLElement | undefined): HTMLButtonElement[] {
    return el
      ? [...el.querySelectorAll<HTMLButtonElement>('[role="option"]')]
      : [];
  }

  // Close a combobox only once focus leaves its whole container (input + list),
  // which keeps it open while the user tabs/arrows through the options and while
  // a click on an option is being processed.
  function comboFocusOut(e: FocusEvent, el: HTMLElement | undefined): boolean {
    const next = e.relatedTarget as Node | null;
    return !next || (!!el && !el.contains(next));
  }

  function onNetworkFocusOut(e: FocusEvent) {
    if (comboFocusOut(e, networkComboEl)) networkDropdownOpen = false;
  }

  function onChassisFocusOut(e: FocusEvent) {
    if (comboFocusOut(e, chassisComboEl)) chassisDropdownOpen = false;
  }

  // Key handling on the combobox input: ArrowDown opens the list and moves focus
  // to the first option; Escape closes it.
  function onNetworkInputKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      networkDropdownOpen = true;
      requestAnimationFrame(() => comboOptions(networkComboEl)[0]?.focus());
    } else if (e.key === 'Escape') {
      networkDropdownOpen = false;
    }
  }

  function onChassisInputKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      chassisDropdownOpen = true;
      requestAnimationFrame(() => comboOptions(chassisComboEl)[0]?.focus());
    } else if (e.key === 'Escape') {
      chassisDropdownOpen = false;
    }
  }

  // Key handling within a listbox: roving Up/Down focus across the options,
  // Escape returns focus to the input and closes the list. The option buttons
  // handle Enter/Space natively.
  function moveComboFocus(
    e: KeyboardEvent,
    el: HTMLElement | undefined,
    input: HTMLInputElement | null,
    close: () => void,
  ) {
    const items = comboOptions(el);
    if (items.length === 0) return;
    const idx = items.indexOf(document.activeElement as HTMLButtonElement);
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        items[(idx + 1) % items.length]?.focus();
        break;
      case 'ArrowUp':
        e.preventDefault();
        if (idx <= 0) input?.focus();
        else items[idx - 1]?.focus();
        break;
      case 'Escape':
        e.preventDefault();
        close();
        input?.focus();
        break;
    }
  }

  function onNetworkListKey(e: KeyboardEvent) {
    moveComboFocus(
      e,
      networkComboEl,
      networkComboEl?.querySelector('input') ?? null,
      () => (networkDropdownOpen = false),
    );
  }

  function onChassisListKey(e: KeyboardEvent) {
    moveComboFocus(
      e,
      chassisComboEl,
      chassisComboEl?.querySelector('input') ?? null,
      () => (chassisDropdownOpen = false),
    );
  }
</script>

<div class="flex h-full flex-col">
  <PageHeader
    eyebrow="Visualize"
    title="Network Topology"
    description="Interactive graph of logical switches, routers, and chassis."
  >
    {#snippet actions()}
      <label
        class="flex cursor-pointer items-center gap-2 font-mono text-2xs tracking-wider text-base-content/60 uppercase select-none"
      >
        <input
          type="checkbox"
          bind:checked={liveUpdates}
          class="checkbox checkbox-sm"
        />
        Live updates
      </label>
      <label
        class="flex cursor-pointer items-center gap-2 font-mono text-2xs tracking-wider text-base-content/60 uppercase select-none"
      >
        <input
          type="checkbox"
          bind:checked={showVMs}
          class="checkbox checkbox-sm"
        />
        Show VM ports
      </label>
      <button
        class="btn border-base-300 btn-ghost font-mono normal-case btn-sm"
        onclick={() => relayoutKey++}
        title="Re-Layout"
      >
        &#x21bb; Layout
      </button>
      <div
        class="relative"
        bind:this={exportMenuEl}
        onfocusout={onExportFocusOut}
      >
        <button
          class="btn border-base-300 btn-ghost font-mono normal-case btn-sm"
          aria-haspopup="menu"
          aria-expanded={exportMenuOpen}
          onclick={() =>
            exportMenuOpen ? closeExportMenu() : openExportMenu()}
          onkeydown={onExportTriggerKey}
        >
          Export &#9662;
        </button>
        {#if exportMenuOpen}
          <ul
            role="menu"
            class="absolute right-0 z-50 mt-1 flex w-44 flex-col rounded border border-base-300 bg-base-100 p-1 font-mono text-sm shadow-lg"
            onkeydown={onExportMenuKey}
          >
            <li role="none">
              <button
                role="menuitem"
                class="w-full rounded px-3 py-1.5 text-left hover:bg-base-300/40"
                onclick={() => runExport(handleExportSVG)}>Download SVG</button
              >
            </li>
            <li role="none">
              <button
                role="menuitem"
                class="w-full rounded px-3 py-1.5 text-left hover:bg-base-300/40"
                onclick={() => runExport(handleExportPNG)}>Download PNG</button
              >
            </li>
            <li role="none">
              <button
                role="menuitem"
                class="w-full rounded px-3 py-1.5 text-left hover:bg-base-300/40"
                onclick={() => runExport(handleExportJSON)}
                >Download JSON</button
              >
            </li>
          </ul>
        {/if}
      </div>
    {/snippet}
  </PageHeader>

  <!-- Filters -->
  <div class="mb-3 flex flex-wrap items-center gap-3">
    <!-- Network focus (searchable) -->
    <div class="flex items-center gap-1.5">
      <span
        class="font-mono text-2xs tracking-wider whitespace-nowrap text-base-content/60 uppercase"
        >Network</span
      >
      <div
        class="relative"
        bind:this={networkComboEl}
        onfocusout={onNetworkFocusOut}
      >
        <input
          type="text"
          role="combobox"
          aria-expanded={networkDropdownOpen}
          aria-controls="network-focus-listbox"
          aria-autocomplete="list"
          bind:value={networkSearch}
          onfocus={() => {
            networkDropdownOpen = true;
          }}
          onkeydown={onNetworkInputKey}
          placeholder="All networks"
          class="input w-48 font-mono input-xs"
        />
        {#if focusNetwork}
          <button
            class="btn absolute top-1/2 right-1 -translate-y-1/2 btn-ghost px-1 btn-xs"
            aria-label="Clear network focus"
            onclick={clearNetwork}>&times;</button
          >
        {/if}
        {#if networkDropdownOpen}
          <ul
            id="network-focus-listbox"
            role="listbox"
            aria-label="Network focus options"
            class="absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded border border-base-300 bg-base-100 font-mono shadow-lg"
            onkeydown={onNetworkListKey}
          >
            <li role="none">
              <button
                role="option"
                aria-selected={!focusNetwork}
                class="w-full px-3 py-1.5 text-left text-xs text-base-content/50 hover:bg-base-300/40"
                onclick={clearNetwork}
              >
                All
              </button>
            </li>
            {#each filteredNetworks as opt (opt.id)}
              <li role="none">
                <button
                  role="option"
                  aria-selected={opt.id === focusNetwork}
                  class="w-full px-3 py-1.5 text-left text-xs hover:bg-base-300/40 {opt.id ===
                  focusNetwork
                    ? 'bg-primary/10 font-semibold'
                    : ''}"
                  onclick={() => selectNetwork(opt.id, opt.label)}
                >
                  {shortLabel(opt.label)}
                  <span class="ml-1 text-base-content/40"
                    >{opt.id.slice(0, 8)}</span
                  >
                </button>
              </li>
            {/each}
            {#if filteredNetworks.length === 0}
              <li role="none" class="px-3 py-1.5 text-xs text-base-content/40">
                No matches
              </li>
            {/if}
          </ul>
        {/if}
      </div>
    </div>

    <!-- Chassis focus (searchable) -->
    <div class="flex items-center gap-1.5">
      <span
        class="font-mono text-2xs tracking-wider whitespace-nowrap text-base-content/60 uppercase"
        >Chassis</span
      >
      <div
        class="relative"
        bind:this={chassisComboEl}
        onfocusout={onChassisFocusOut}
      >
        <input
          type="text"
          role="combobox"
          aria-expanded={chassisDropdownOpen}
          aria-controls="chassis-focus-listbox"
          aria-autocomplete="list"
          bind:value={chassisSearch}
          onfocus={() => {
            chassisDropdownOpen = true;
          }}
          onkeydown={onChassisInputKey}
          placeholder="All chassis"
          class="input w-48 font-mono input-xs"
        />
        {#if focusChassis}
          <button
            class="btn absolute top-1/2 right-1 -translate-y-1/2 btn-ghost px-1 btn-xs"
            aria-label="Clear chassis focus"
            onclick={clearChassis}>&times;</button
          >
        {/if}
        {#if chassisDropdownOpen}
          <ul
            id="chassis-focus-listbox"
            role="listbox"
            aria-label="Chassis focus options"
            class="absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded border border-base-300 bg-base-100 font-mono shadow-lg"
            onkeydown={onChassisListKey}
          >
            <li role="none">
              <button
                role="option"
                aria-selected={!focusChassis}
                class="w-full px-3 py-1.5 text-left text-xs text-base-content/50 hover:bg-base-300/40"
                onclick={clearChassis}
              >
                All
              </button>
            </li>
            {#each filteredChassis as opt (opt.id)}
              <li role="none">
                <button
                  role="option"
                  aria-selected={opt.id === focusChassis}
                  class="w-full px-3 py-1.5 text-left text-xs hover:bg-base-300/40 {opt.id ===
                  focusChassis
                    ? 'bg-primary/10 font-semibold'
                    : ''}"
                  onclick={() => selectChassis(opt.id, opt.label, opt.gw)}
                >
                  {opt.label}
                  {#if opt.gw}<span class="ml-1 font-medium text-purple-500"
                      >(GW)</span
                    >{/if}
                </button>
              </li>
            {/each}
            {#if filteredChassis.length === 0}
              <li role="none" class="px-3 py-1.5 text-xs text-base-content/40">
                No matches
              </li>
            {/if}
          </ul>
        {/if}
      </div>
    </div>

    <!-- Search highlight -->
    <div class="flex items-center gap-1.5">
      <span
        class="font-mono text-2xs tracking-wider text-base-content/60 uppercase"
        >Search</span
      >
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Name, UUID, IP..."
        class="input w-48 font-mono input-xs"
      />
    </div>

    <!-- Clear -->
    {#if focusNetwork || focusChassis || searchQuery}
      <button
        class="btn border-base-300 btn-ghost font-mono normal-case btn-xs"
        onclick={clearFilters}>Clear filters</button
      >
    {/if}
  </div>

  <!-- Legend -->
  <div
    class="mb-3 flex gap-4 font-mono text-2xs tracking-wider text-base-content/55 uppercase"
  >
    <div class="flex items-center gap-1.5">
      <span class="inline-block h-3 w-4 rounded-xs bg-blue-500 opacity-85"
      ></span>
      Switch
    </div>
    <div class="flex items-center gap-1.5">
      <span class="inline-block h-3 w-3 rotate-45 bg-green-500 opacity-85"
      ></span>
      Router
    </div>
    <div class="flex items-center gap-1.5">
      <span class="inline-block h-3 w-4 rounded-xs bg-gray-500 opacity-60"
      ></span>
      Chassis
    </div>
    <div class="flex items-center gap-1.5">
      <span
        class="inline-block h-3 w-4 rounded-xs opacity-85"
        style="background: #7c3aed;"
      ></span>
      Gateway
    </div>
    {#if showVMs}
      <div class="flex items-center gap-1.5">
        <span
          class="inline-block h-2.5 w-2.5 rounded-full bg-amber-400 opacity-85"
        ></span>
        VM Port
      </div>
    {/if}
  </div>

  <DataState
    {loading}
    {error}
    empty={allNodes.length === 0}
    emptyMessage="no topology data available"
  >
    <div
      class="relative flex-1 overflow-hidden rounded border border-base-300 bg-base-100"
      style="min-height: 500px"
    >
      <TopologyGraph
        {nodes}
        {edges}
        {searchQuery}
        {relayoutKey}
        bind:svgRef={topologySvgRef}
      />
      {#if liveUpdates}
        <TopologyEventStream
          events={liveEvents}
          onSelect={showEventDetail}
          onClear={() => (liveEvents = [])}
        />
      {/if}
    </div>
  </DataState>
</div>

{#if selectedEvent}
  <EventDetailPanel
    event={selectedEvent}
    onClose={() => (selectedEvent = null)}
  />
{/if}
