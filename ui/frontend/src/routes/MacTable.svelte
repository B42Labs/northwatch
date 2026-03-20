<script lang="ts">
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { SvelteMap } from 'svelte/reactivity';

  interface MacBinding {
    _uuid: string;
    datapath: string;
    ip: string;
    logical_port: string;
    mac: string;
    timestamp: number;
  }

  interface DatapathBinding {
    _uuid: string;
    external_ids: Record<string, string>;
    tunnel_key: number;
    [key: string]: unknown;
  }

  interface DatapathGroup {
    datapath: DatapathBinding;
    name: string;
    type: 'router' | 'switch' | 'unknown';
    entries: MacBinding[];
  }

  let loading = $state(true);
  let error = $state('');

  let macBindings: MacBinding[] = $state([]);
  let datapaths: DatapathBinding[] = $state([]);

  let globalSearch = $state('');

  async function fetchJson<T>(path: string): Promise<T> {
    const res = await fetch(path);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const [bindings, dps] = await Promise.all([
        fetchJson<MacBinding[]>('/api/v1/sb/mac-bindings'),
        fetchJson<DatapathBinding[]>('/api/v1/sb/datapath-bindings'),
      ]);
      macBindings = bindings;
      datapaths = dps;
    } catch (e) {
      error =
        e instanceof Error ? e.message : 'Failed to load MAC binding data';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load();
  });

  // Build lookup map from datapath UUID to DatapathBinding
  let datapathByUuid = $derived(new Map(datapaths.map((dp) => [dp._uuid, dp])));

  // Determine datapath name and type from external_ids
  function getDatapathInfo(dp: DatapathBinding | undefined): {
    name: string;
    type: 'router' | 'switch' | 'unknown';
  } {
    if (!dp) return { name: 'Unknown Datapath', type: 'unknown' };
    const extIds = dp.external_ids ?? {};
    const name =
      extIds['name'] ||
      extIds['logical-router'] ||
      extIds['logical-switch'] ||
      dp._uuid.slice(0, 8);
    const type: 'router' | 'switch' | 'unknown' = extIds['logical-router']
      ? 'router'
      : extIds['logical-switch']
        ? 'switch'
        : 'unknown';
    return { name, type };
  }

  // Filter MAC bindings by global search
  let filteredBindings = $derived.by(() => {
    if (!globalSearch.trim()) return macBindings;
    const q = globalSearch.trim().toLowerCase();
    return macBindings.filter(
      (entry) =>
        entry.ip.toLowerCase().includes(q) ||
        entry.mac.toLowerCase().includes(q) ||
        entry.logical_port.toLowerCase().includes(q),
    );
  });

  // Group filtered bindings by datapath
  let datapathGroups = $derived.by((): DatapathGroup[] => {
    const groupMap = new SvelteMap<string, MacBinding[]>();
    for (const entry of filteredBindings) {
      const key = entry.datapath;
      const list = groupMap.get(key);
      if (list) {
        list.push(entry);
      } else {
        groupMap.set(key, [entry]);
      }
    }

    const groups: DatapathGroup[] = [];
    for (const [dpUuid, entries] of groupMap) {
      const dp = datapathByUuid.get(dpUuid);
      const { name, type } = getDatapathInfo(dp);
      groups.push({
        datapath: dp ?? {
          _uuid: dpUuid,
          external_ids: {},
          tunnel_key: 0,
        },
        name,
        type,
        entries: entries.sort((a, b) => a.ip.localeCompare(b.ip)),
      });
    }

    return groups.sort((a, b) => a.name.localeCompare(b.name));
  });

  // Summary counts
  let totalEntries = $derived(filteredBindings.length);
  let totalDatapaths = $derived(datapathGroups.length);

  function typeBadgeClass(type: 'router' | 'switch' | 'unknown'): string {
    switch (type) {
      case 'router':
        return 'bg-green-600 text-white';
      case 'switch':
        return 'bg-blue-600 text-white';
      default:
        return 'badge-ghost';
    }
  }

  function typeBadgeLabel(type: 'router' | 'switch' | 'unknown'): string {
    switch (type) {
      case 'router':
        return 'Router';
      case 'switch':
        return 'Switch';
      default:
        return 'Unknown';
    }
  }
</script>

<div>
  <!-- Header -->
  <div class="mb-4">
    <h1 class="text-xl font-bold">MAC / ARP Table</h1>
    <p class="text-sm text-base-content/60">
      Learned MAC-to-IP bindings from OVN Southbound DB, grouped by datapath
    </p>
  </div>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else}
    <!-- Summary + Search bar -->
    <div class="mb-4 flex flex-wrap items-center gap-4">
      <div>
        <input
          type="text"
          placeholder="Search by IP, MAC or port..."
          class="input input-sm input-bordered w-72"
          bind:value={globalSearch}
        />
      </div>
      <div
        class="stats stats-horizontal ml-auto border border-base-300 bg-base-100 shadow-sm"
      >
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">MAC Entries</div>
          <div class="stat-value text-lg">{totalEntries}</div>
          {#if globalSearch.trim() && totalEntries !== macBindings.length}
            <div class="stat-desc text-xs">of {macBindings.length} total</div>
          {/if}
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Datapaths</div>
          <div class="stat-value text-lg">{totalDatapaths}</div>
        </div>
      </div>
    </div>

    {#if datapathGroups.length === 0}
      <div class="py-8 text-center text-base-content/50">
        {#if globalSearch.trim()}
          No MAC entries match the search criteria
        {:else}
          No MAC bindings found
        {/if}
      </div>
    {:else}
      <div class="flex flex-col gap-6">
        {#each datapathGroups as group}
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body p-4">
              <!-- Datapath header -->
              <div class="flex items-center gap-2">
                <span
                  class="inline-block h-3 w-3 rounded-sm opacity-85"
                  class:bg-green-500={group.type === 'router'}
                  class:rotate-45={group.type === 'router'}
                  class:bg-blue-500={group.type === 'switch'}
                  class:rounded-full={group.type === 'switch'}
                  class:bg-gray-400={group.type === 'unknown'}
                ></span>
                <h2 class="card-title text-sm">
                  {group.name}
                </h2>
                <span class="badge badge-sm {typeBadgeClass(group.type)}">
                  {typeBadgeLabel(group.type)}
                </span>
                <span class="text-xs text-base-content/40">
                  {group.datapath._uuid.slice(0, 8)}
                </span>
                <span class="badge badge-outline badge-sm">
                  {group.entries.length}
                  {group.entries.length === 1 ? 'entry' : 'entries'}
                </span>
              </div>

              <!-- MAC entries table -->
              <div class="mt-2 overflow-x-auto">
                <table class="table table-zebra table-xs">
                  <thead>
                    <tr>
                      <th>IP Address</th>
                      <th>MAC Address</th>
                      <th>Logical Port</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each group.entries as entry}
                      <tr>
                        <td class="font-mono text-xs">{entry.ip}</td>
                        <td class="font-mono text-xs">{entry.mac}</td>
                        <td class="text-xs">{entry.logical_port || '-'}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
