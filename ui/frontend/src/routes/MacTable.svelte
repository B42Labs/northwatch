<script lang="ts">
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Card from '../components/ui/Card.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import type { Variant } from '../lib/status';
  import { SvelteMap } from 'svelte/reactivity';
  import { get } from '../lib/api';

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

  async function load() {
    loading = true;
    error = '';
    try {
      const [bindings, dps] = await Promise.all([
        get<MacBinding[]>('/api/v1/sb/mac-bindings'),
        get<DatapathBinding[]>('/api/v1/sb/datapath-bindings'),
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

  function typeVariant(type: 'router' | 'switch' | 'unknown'): Variant {
    switch (type) {
      case 'router':
        return 'success';
      case 'switch':
        return 'info';
      default:
        return 'ghost';
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

<PageHeader
  eyebrow="Southbound"
  title="MAC / ARP Table"
  description="Learned MAC-to-IP bindings from OVN Southbound DB, grouped by datapath."
>
  {#snippet actions()}
    <StatTiles
      tiles={[
        {
          label: 'MAC Entries',
          value: totalEntries,
          hint:
            globalSearch.trim() && totalEntries !== macBindings.length
              ? `of ${macBindings.length} total`
              : undefined,
        },
        { label: 'Datapaths', value: totalDatapaths },
      ]}
    />
  {/snippet}
</PageHeader>

<DataState {loading} {error}>
  <!-- Search bar -->
  <div class="mb-3 flex flex-wrap items-center gap-2">
    <FilterInput
      bind:value={globalSearch}
      placeholder="filter by IP, MAC or port…"
      width="w-72"
    />
  </div>

  {#if datapathGroups.length === 0}
    <div class="py-8 text-center font-mono text-sm text-base-content/40">
      <span class="text-base-content/30">//</span>
      {#if globalSearch.trim()}
        no MAC entries match the search criteria
      {:else}
        no MAC bindings found
      {/if}
    </div>
  {:else}
    <div class="flex flex-col gap-4">
      {#each datapathGroups as group (group.datapath._uuid)}
        <Card title={group.name} subtitle={group.datapath._uuid.slice(0, 8)}>
          {#snippet actions()}
            <Badge
              text={typeBadgeLabel(group.type)}
              variant={typeVariant(group.type)}
            />
            <Badge
              text="{group.entries.length} {group.entries.length === 1
                ? 'entry'
                : 'entries'}"
              variant="neutral"
              outline
            />
          {/snippet}

          <!-- MAC entries table -->
          <div class="overflow-x-auto rounded border border-base-300">
            <table class="table table-xs font-mono">
              <thead>
                <tr>
                  <th
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                    >IP Address</th
                  >
                  <th
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                    >MAC Address</th
                  >
                  <th
                    class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                    >Logical Port</th
                  >
                </tr>
              </thead>
              <tbody>
                {#each group.entries as entry, i (i)}
                  <tr class="border-base-300/60">
                    <td class="text-xs">{entry.ip}</td>
                    <td class="text-xs">{entry.mac}</td>
                    <td class="text-xs text-base-content/70"
                      >{entry.logical_port || '-'}</td
                    >
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </Card>
      {/each}
    </div>
  {/if}
</DataState>
