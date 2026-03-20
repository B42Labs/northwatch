<script lang="ts">
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';

  // --- Types ---

  interface HaChassisEntry {
    _uuid: string;
    chassis: string | null;
    priority: number;
    external_ids: Record<string, string>;
  }

  interface HaChassisGroup {
    _uuid: string;
    name: string;
    ha_chassis: string[];
    ref_chassis: string[];
    external_ids: Record<string, string>;
  }

  interface ChassisRecord {
    _uuid: string;
    name: string;
    hostname: string;
  }

  interface PortBindingRecord {
    _uuid: string;
    type: string;
    logical_port: string;
    chassis: string | null;
    ha_chassis_group: string | null;
  }

  interface ResolvedChassisEntry {
    uuid: string;
    chassisUuid: string | null;
    chassisName: string;
    hostname: string;
    priority: number;
    isActive: boolean;
  }

  interface ResolvedGroup {
    uuid: string;
    name: string;
    chassisChain: ResolvedChassisEntry[];
    crPortName: string | null;
    activeChassis: string | null;
  }

  // --- Fetch helper ---

  async function fetchJson<T>(path: string): Promise<T> {
    const res = await fetch(path);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  // --- State ---

  let loading = $state(true);
  let error = $state('');
  let groups: ResolvedGroup[] = $state([]);
  let totalChassisInvolved = $state(0);
  let searchQuery = $state('');

  // --- Filtered groups ---

  let filteredGroups = $derived.by(() => {
    if (!searchQuery.trim()) return groups;
    const q = searchQuery.toLowerCase();
    return groups.filter((g) => {
      if (g.name.toLowerCase().includes(q)) return true;
      if (g.crPortName?.toLowerCase().includes(q)) return true;
      return g.chassisChain.some(
        (c) =>
          c.chassisName.toLowerCase().includes(q) ||
          c.hostname.toLowerCase().includes(q),
      );
    });
  });

  // --- Load data ---

  async function load() {
    loading = true;
    error = '';
    try {
      const [haGroups, haChassis, chassisList, portBindings] =
        await Promise.all([
          fetchJson<HaChassisGroup[]>('/api/v1/sb/ha-chassis-groups'),
          fetchJson<HaChassisEntry[]>('/api/v1/sb/ha-chassis'),
          fetchJson<ChassisRecord[]>('/api/v1/sb/chassis'),
          fetchJson<PortBindingRecord[]>('/api/v1/sb/port-bindings'),
        ]);

      // Build lookup maps
      const haChassisMap = new SvelteMap<string, HaChassisEntry>();
      for (const hc of haChassis) {
        haChassisMap.set(hc._uuid, hc);
      }

      const chassisMap = new SvelteMap<string, ChassisRecord>();
      for (const ch of chassisList) {
        chassisMap.set(ch._uuid, ch);
      }

      // Find chassisredirect port bindings and map ha_chassis_group UUID -> active chassis UUID
      const crBindings = portBindings.filter(
        (pb) => pb.type === 'chassisredirect',
      );
      const haGroupToActiveChassis = new SvelteMap<string, string>();
      const haGroupToCrPort = new SvelteMap<string, string>();
      for (const pb of crBindings) {
        if (pb.ha_chassis_group && pb.chassis) {
          haGroupToActiveChassis.set(pb.ha_chassis_group, pb.chassis);
          haGroupToCrPort.set(pb.ha_chassis_group, pb.logical_port);
        }
      }

      // Collect unique chassis involved
      const chassisUuidsInvolved = new SvelteSet<string>();

      // Resolve each HA group
      const resolved: ResolvedGroup[] = haGroups.map((group) => {
        const activeChassis = haGroupToActiveChassis.get(group._uuid) ?? null;
        const crPortName = haGroupToCrPort.get(group._uuid) ?? null;

        // Resolve HA chassis entries for this group
        const chainEntries: ResolvedChassisEntry[] = group.ha_chassis
          .map((hcUuid) => {
            const hc = haChassisMap.get(hcUuid);
            if (!hc) return null;

            const chassisRecord = hc.chassis
              ? chassisMap.get(hc.chassis)
              : null;

            if (hc.chassis) {
              chassisUuidsInvolved.add(hc.chassis);
            }

            return {
              uuid: hc._uuid,
              chassisUuid: hc.chassis,
              chassisName: chassisRecord?.name ?? hc.chassis ?? 'unknown',
              hostname: chassisRecord?.hostname ?? '',
              priority: hc.priority,
              isActive: hc.chassis === activeChassis && activeChassis !== null,
            } satisfies ResolvedChassisEntry;
          })
          .filter((e): e is ResolvedChassisEntry => e !== null)
          .sort((a, b) => b.priority - a.priority);

        return {
          uuid: group._uuid,
          name: group.name,
          chassisChain: chainEntries,
          crPortName,
          activeChassis,
        };
      });

      // Sort groups by name
      resolved.sort((a, b) => a.name.localeCompare(b.name));

      groups = resolved;
      totalChassisInvolved = chassisUuidsInvolved.size;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load HA data';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load();
  });

  function shortName(name: string): string {
    if (name.length <= 32) return name;
    return name.slice(0, 29) + '...';
  }
</script>

<div>
  <!-- Header -->
  <div class="mb-4">
    <h1 class="text-xl font-bold">HA Failover</h1>
    <p class="text-sm text-base-content/60">
      HA Chassis Groups and gateway chassis failover chains
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
          bind:value={searchQuery}
          placeholder="Filter by group name, chassis..."
          class="input input-sm input-bordered w-72"
        />
      </div>
      <div
        class="stats stats-horizontal ml-auto border border-base-300 bg-base-100 shadow-sm"
      >
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">HA Groups</div>
          <div class="stat-value text-lg">{groups.length}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Chassis Involved</div>
          <div class="stat-value text-lg">{totalChassisInvolved}</div>
        </div>
      </div>
    </div>

    {#if groups.length === 0}
      <div class="py-8 text-center text-base-content/50">
        No HA Chassis Groups found
      </div>
    {:else if filteredGroups.length === 0}
      <div class="py-8 text-center text-base-content/50">
        No groups match the filter
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-4">
        {#each filteredGroups as group (group.uuid)}
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body p-4">
              <!-- Group header -->
              <div
                class="mb-3 flex flex-wrap items-start justify-between gap-2"
              >
                <div>
                  <h2 class="card-title text-sm font-semibold">
                    {shortName(group.name)}
                  </h2>
                  {#if group.crPortName}
                    <p class="mt-0.5 text-xs text-base-content/50">
                      CR port: {group.crPortName}
                    </p>
                  {/if}
                </div>
                <div class="flex items-center gap-2">
                  <span class="badge badge-outline badge-sm">
                    {group.chassisChain.length} chassis
                  </span>
                  {#if group.activeChassis}
                    <span class="badge badge-success badge-sm">has active</span>
                  {:else}
                    <span class="badge badge-warning badge-sm">no active</span>
                  {/if}
                </div>
              </div>

              <!-- Chassis failover chain -->
              {#if group.chassisChain.length === 0}
                <div class="py-2 text-xs text-base-content/40">
                  No chassis entries in this group
                </div>
              {:else}
                <div class="flex flex-wrap items-center gap-0">
                  {#each group.chassisChain as entry, idx (entry.uuid)}
                    <!-- Chassis box -->
                    <div
                      class="relative flex min-w-[140px] flex-col rounded-lg border-2 px-3 py-2 {entry.isActive
                        ? 'border-success bg-success/5'
                        : 'border-base-300 bg-base-200/50'}"
                    >
                      <!-- Priority badge -->
                      <div class="mb-1 flex items-center justify-between gap-2">
                        <span
                          class="badge badge-sm font-mono {entry.isActive
                            ? 'badge-success'
                            : 'badge-ghost'}"
                        >
                          P{entry.priority}
                        </span>
                        {#if entry.isActive}
                          <span class="badge badge-success badge-sm"
                            >ACTIVE</span
                          >
                        {:else if idx === 0 && !group.activeChassis}
                          <span class="badge badge-warning badge-sm"
                            >STANDBY</span
                          >
                        {:else}
                          <span class="badge badge-ghost badge-sm">STANDBY</span
                          >
                        {/if}
                      </div>
                      <!-- Chassis name -->
                      <div
                        class="text-sm font-medium"
                        title={entry.chassisName}
                      >
                        {shortName(entry.chassisName)}
                      </div>
                      {#if entry.hostname}
                        <div
                          class="text-xs text-base-content/50"
                          title={entry.hostname}
                        >
                          {entry.hostname}
                        </div>
                      {/if}
                    </div>

                    <!-- Arrow connector between chassis boxes -->
                    {#if idx < group.chassisChain.length - 1}
                      <div class="flex items-center px-1 text-base-content/30">
                        <svg
                          xmlns="http://www.w3.org/2000/svg"
                          class="h-5 w-5"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M9 5l7 7-7 7"
                          />
                        </svg>
                      </div>
                    {/if}
                  {/each}
                </div>

                <!-- Legend for this card -->
                <div class="mt-2 text-xs text-base-content/40">
                  Ordered by priority (highest first). Highest priority with
                  bound CR port = active gateway.
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
