<script lang="ts">
  import {
    getTopology,
    type TopologyNode,
    type TopologyEdge,
  } from '../lib/api';
  import TopologyGraph from '../components/topology/TopologyGraph.svelte';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { subscribeToTables } from '../lib/eventStore';

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
    if (!liveUpdates) return;
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
      () => {
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

  function handleNetworkBlur() {
    // Delay to allow click on option
    setTimeout(() => {
      networkDropdownOpen = false;
    }, 200);
  }

  function handleChassisBlur() {
    setTimeout(() => {
      chassisDropdownOpen = false;
    }, 200);
  }
</script>

<div class="flex h-full flex-col">
  <!-- Header -->
  <div class="mb-3 flex items-center justify-between">
    <div>
      <h1 class="text-xl font-bold">Network Topology</h1>
      <p class="text-sm text-base-content/60">
        Interactive graph of logical switches, routers, and chassis
      </p>
    </div>
    <div class="flex items-center gap-3">
      <label class="flex cursor-pointer select-none items-center gap-2 text-sm">
        <input
          type="checkbox"
          bind:checked={liveUpdates}
          class="checkbox checkbox-sm"
        />
        Live updates
      </label>
      <label class="flex cursor-pointer select-none items-center gap-2 text-sm">
        <input
          type="checkbox"
          bind:checked={showVMs}
          class="checkbox checkbox-sm"
        />
        Show VM ports
      </label>
      <button
        class="btn btn-ghost btn-sm"
        onclick={() => relayoutKey++}
        title="Re-Layout"
      >
        &#x21bb; Layout
      </button>
    </div>
  </div>

  <!-- Filters -->
  <div class="mb-3 flex flex-wrap items-center gap-3">
    <!-- Network focus (searchable) -->
    <div class="flex items-center gap-1.5">
      <span class="whitespace-nowrap text-xs text-base-content/60">Network</span
      >
      <div class="relative">
        <input
          type="text"
          bind:value={networkSearch}
          onfocus={() => {
            networkDropdownOpen = true;
          }}
          onblur={handleNetworkBlur}
          placeholder="All networks"
          class="input input-xs input-bordered w-48"
        />
        {#if focusNetwork}
          <button
            class="btn btn-ghost btn-xs absolute right-1 top-1/2 -translate-y-1/2 px-1"
            onclick={clearNetwork}>&times;</button
          >
        {/if}
        {#if networkDropdownOpen}
          <ul
            class="absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded-lg border border-base-300 bg-base-100 shadow-lg"
          >
            <li>
              <button
                class="w-full px-3 py-1.5 text-left text-xs text-base-content/50 hover:bg-base-200"
                onclick={clearNetwork}
              >
                All
              </button>
            </li>
            {#each filteredNetworks as opt}
              <li>
                <button
                  class="w-full px-3 py-1.5 text-left text-xs hover:bg-base-200 {opt.id ===
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
              <li class="px-3 py-1.5 text-xs text-base-content/40">
                No matches
              </li>
            {/if}
          </ul>
        {/if}
      </div>
    </div>

    <!-- Chassis focus (searchable) -->
    <div class="flex items-center gap-1.5">
      <span class="whitespace-nowrap text-xs text-base-content/60">Chassis</span
      >
      <div class="relative">
        <input
          type="text"
          bind:value={chassisSearch}
          onfocus={() => {
            chassisDropdownOpen = true;
          }}
          onblur={handleChassisBlur}
          placeholder="All chassis"
          class="input input-xs input-bordered w-48"
        />
        {#if focusChassis}
          <button
            class="btn btn-ghost btn-xs absolute right-1 top-1/2 -translate-y-1/2 px-1"
            onclick={clearChassis}>&times;</button
          >
        {/if}
        {#if chassisDropdownOpen}
          <ul
            class="absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded-lg border border-base-300 bg-base-100 shadow-lg"
          >
            <li>
              <button
                class="w-full px-3 py-1.5 text-left text-xs text-base-content/50 hover:bg-base-200"
                onclick={clearChassis}
              >
                All
              </button>
            </li>
            {#each filteredChassis as opt}
              <li>
                <button
                  class="w-full px-3 py-1.5 text-left text-xs hover:bg-base-200 {opt.id ===
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
              <li class="px-3 py-1.5 text-xs text-base-content/40">
                No matches
              </li>
            {/if}
          </ul>
        {/if}
      </div>
    </div>

    <!-- Search highlight -->
    <div class="flex items-center gap-1.5">
      <span class="text-xs text-base-content/60">Search</span>
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Name, UUID, IP..."
        class="input input-xs input-bordered w-48"
      />
    </div>

    <!-- Clear -->
    {#if focusNetwork || focusChassis || searchQuery}
      <button class="btn btn-ghost btn-xs" onclick={clearFilters}
        >Clear filters</button
      >
    {/if}
  </div>

  <!-- Legend -->
  <div class="mb-3 flex gap-4 text-xs text-base-content/60">
    <div class="flex items-center gap-1.5">
      <span class="inline-block h-3 w-4 rounded-sm bg-blue-500 opacity-85"
      ></span>
      Switch
    </div>
    <div class="flex items-center gap-1.5">
      <span class="inline-block h-3 w-3 rotate-45 bg-green-500 opacity-85"
      ></span>
      Router
    </div>
    <div class="flex items-center gap-1.5">
      <span class="inline-block h-3 w-4 rounded-sm bg-gray-500 opacity-60"
      ></span>
      Chassis
    </div>
    <div class="flex items-center gap-1.5">
      <span
        class="inline-block h-3 w-4 rounded-sm opacity-85"
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

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else if allNodes.length === 0}
    <div class="py-8 text-center text-base-content/50">
      No topology data available
    </div>
  {:else}
    <div
      class="flex-1 overflow-hidden rounded-lg border border-base-300 bg-base-100"
      style="min-height: 500px"
    >
      <TopologyGraph {nodes} {edges} {searchQuery} {relayoutKey} />
    </div>
  {/if}
</div>
