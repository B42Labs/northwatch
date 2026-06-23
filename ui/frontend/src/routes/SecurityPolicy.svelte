<script lang="ts">
  import { listTable } from '../lib/api';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import type { Variant } from '../lib/status';
  import { subscribeToTables } from '../lib/eventStore';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';

  // --- Types ---

  interface ResolvedACL {
    uuid: string;
    direction: string;
    action: string;
    match: string;
    priority: number;
  }

  interface ResolvedPortGroup {
    uuid: string;
    name: string;
    displayName: string;
    ports: { uuid: string; name: string }[];
    acls: ResolvedACL[];
  }

  // --- State ---

  let loading = $state(true);
  let error = $state('');
  let portGroups: ResolvedPortGroup[] = $state([]);
  let standaloneACLs: ResolvedACL[] = $state([]);
  let expandedGroups = new SvelteSet<string>();
  let filterText = $state('');
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  // --- Helpers ---

  /** Convert a raw ACL record to a resolved ACL */
  function toResolvedACL(raw: Record<string, unknown>): ResolvedACL {
    return {
      uuid: (raw._uuid as string) || '',
      direction: (raw.direction as string) || '',
      action: (raw.action as string) || '',
      match: (raw.match as string) || '',
      priority: (raw.priority as number) || 0,
    };
  }

  /** Clean up Neutron-style port group names like pg_678091eb_cae5_4e6a_b038_e72347966983 */
  function cleanName(name: string): string {
    const neutronMatch = name.match(
      /^pg_([0-9a-f]{8})_([0-9a-f]{4})_([0-9a-f]{4})_([0-9a-f]{4})_([0-9a-f]{12})$/,
    );
    if (neutronMatch) {
      return `${neutronMatch[1]}-${neutronMatch[2]}-${neutronMatch[3]}-${neutronMatch[4]}-${neutronMatch[5]}`;
    }
    // Also handle neutron- prefix style
    const neutronPrefixMatch = name.match(/^neutron-pg-([a-f0-9-]+)$/);
    if (neutronPrefixMatch) {
      return neutronPrefixMatch[1];
    }
    return name;
  }

  function directionLabel(dir: string): string {
    return dir === 'to-lport' ? 'ingress' : 'egress';
  }

  function directionVariant(dir: string): Variant {
    return dir === 'to-lport' ? 'info' : 'warning';
  }

  function aclActionVariant(action: string): Variant {
    if (
      action === 'allow' ||
      action === 'allow-related' ||
      action === 'allow-stateless'
    ) {
      return 'success';
    }
    if (action === 'drop') return 'error';
    if (action === 'reject') return 'warning';
    return 'neutral';
  }

  function toggleGroup(uuid: string) {
    if (expandedGroups.has(uuid)) {
      expandedGroups.delete(uuid);
    } else {
      expandedGroups.add(uuid);
    }
  }

  // --- Filtered data ---

  let filteredPortGroups = $derived.by(() => {
    if (!filterText) return portGroups;
    const q = filterText.toLowerCase();
    return portGroups.filter((pg) => {
      if (pg.displayName.toLowerCase().includes(q)) return true;
      if (pg.name.toLowerCase().includes(q)) return true;
      if (pg.ports.some((p) => p.name.toLowerCase().includes(q))) return true;
      if (pg.acls.some((acl) => acl.match.toLowerCase().includes(q)))
        return true;
      return false;
    });
  });

  let filteredStandaloneACLs = $derived.by(() => {
    if (!filterText) return standaloneACLs;
    const q = filterText.toLowerCase();
    return standaloneACLs.filter(
      (acl) =>
        acl.match.toLowerCase().includes(q) ||
        acl.action.toLowerCase().includes(q) ||
        acl.direction.toLowerCase().includes(q),
    );
  });

  let totalACLs = $derived(
    portGroups.reduce((sum, pg) => sum + pg.acls.length, 0) +
      standaloneACLs.length,
  );

  // --- Data loading ---

  async function load() {
    try {
      const [rawACLs, rawPortGroups, rawLSPs] = await Promise.all([
        listTable('nb', 'acls'),
        listTable('nb', 'port-groups'),
        listTable('nb', 'logical-switch-ports'),
      ]);

      // Build UUID -> LSP name lookup
      const lspMap = new SvelteMap<string, string>();
      for (const lsp of rawLSPs) {
        lspMap.set(lsp._uuid as string, lsp.name as string);
      }

      // Build UUID -> raw ACL lookup
      const aclMap = new SvelteMap<string, Record<string, unknown>>();
      for (const acl of rawACLs) {
        aclMap.set(acl._uuid as string, acl);
      }

      // Track which ACL UUIDs belong to a port group
      const assignedACLs = new SvelteSet<string>();

      // Resolve port groups
      const resolved: ResolvedPortGroup[] = rawPortGroups
        .map((pg) => {
          const pgPorts = (pg.ports ?? []) as string[];
          const pgACLs = (pg.acls ?? []) as string[];
          const pgName = (pg.name as string) || '';
          const pgUuid = (pg._uuid as string) || '';

          const resolvedPorts = pgPorts.map((uuid) => ({
            uuid,
            name: lspMap.get(uuid) || uuid.slice(0, 8),
          }));

          const resolvedACLs: ResolvedACL[] = [];
          for (const aclUuid of pgACLs) {
            const raw = aclMap.get(aclUuid);
            if (raw) {
              resolvedACLs.push(toResolvedACL(raw));
              assignedACLs.add(aclUuid);
            }
          }

          // Sort ACLs by priority descending
          resolvedACLs.sort((a, b) => b.priority - a.priority);

          return {
            uuid: pgUuid,
            name: pgName,
            displayName: cleanName(pgName),
            ports: resolvedPorts.sort((a, b) => a.name.localeCompare(b.name)),
            acls: resolvedACLs,
          };
        })
        .sort((a, b) => a.displayName.localeCompare(b.displayName));

      portGroups = resolved;

      // Standalone ACLs: not assigned to any port group
      standaloneACLs = rawACLs
        .filter((acl) => !assignedACLs.has(acl._uuid as string))
        .map(toResolvedACL)
        .sort((a, b) => b.priority - a.priority);

      error = '';
    } catch (e) {
      error =
        e instanceof Error ? e.message : 'Failed to load security policy data';
    } finally {
      loading = false;
    }
  }

  // Initial load
  $effect(() => {
    load();
  });

  // Live updates: re-fetch when ACLs or Port Groups change
  $effect(() => {
    const unsubscribe = subscribeToTables(
      'nb',
      ['ACL', 'Port_Group', 'Logical_Switch_Port'],
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
</script>

<PageHeader
  eyebrow="Security"
  title="Security Policy"
  description="ACL rules and Port Groups — showing how traffic is filtered across logical ports."
>
  {#snippet actions()}
    <StatTiles
      tiles={[
        { label: 'Port Groups', value: portGroups.length },
        { label: 'ACL Rules', value: totalACLs },
        ...(standaloneACLs.length > 0
          ? [{ label: 'Standalone', value: standaloneACLs.length }]
          : []),
      ]}
    />
  {/snippet}
</PageHeader>

<DataState {loading} {error}>
  <!-- Filter bar -->
  <div class="mb-3 flex flex-wrap items-center gap-2">
    <FilterInput
      bind:value={filterText}
      placeholder="filter by name, port, or match…"
      width="w-72"
    />
  </div>

  <!-- Port Group panels -->
  {#if filteredPortGroups.length > 0}
    <div class="flex flex-col gap-3">
      {#each filteredPortGroups as pg (pg.uuid)}
        <section class="overflow-hidden rounded border border-base-300 bg-base-100">
          <!-- Panel header (collapse toggle) -->
          <button
            class="flex w-full cursor-pointer items-center gap-3 border-b border-base-300 bg-base-200/40 px-3 py-2 text-left hover:bg-base-200/70"
            onclick={() => toggleGroup(pg.uuid)}
          >
            <span
              class="select-none text-base-content/50 transition-transform duration-200"
              class:rotate-90={expandedGroups.has(pg.uuid)}
            >
              &#9654;
            </span>
            <div class="min-w-0 flex-1">
              <h2 class="truncate font-mono text-sm font-semibold text-base-content/85">
                {pg.displayName}
              </h2>
              {#if pg.displayName !== pg.name}
                <p class="truncate font-mono text-2xs text-base-content/40">
                  {pg.name}
                </p>
              {/if}
            </div>
            <div class="flex flex-shrink-0 items-center gap-1.5">
              <Badge
                text="{pg.ports.length} port{pg.ports.length !== 1 ? 's' : ''}"
                variant="ghost"
              />
              <Badge
                text="{pg.acls.length} ACL{pg.acls.length !== 1 ? 's' : ''}"
                variant="ghost"
              />
            </div>
          </button>

          <!-- Expanded content -->
          {#if expandedGroups.has(pg.uuid)}
            <div class="p-3">
              <!-- Member ports -->
              {#if pg.ports.length > 0}
                <div class="mb-3">
                  <h3
                    class="mb-1 font-mono text-2xs font-semibold uppercase tracking-wider text-base-content/55"
                  >
                    Member Ports
                  </h3>
                  <div class="flex flex-wrap gap-1.5">
                    {#each pg.ports as port (port.uuid)}
                      <Badge text={port.name} variant="neutral" outline />
                    {/each}
                  </div>
                </div>
              {/if}

              <!-- ACL rules table -->
              {#if pg.acls.length > 0}
                <div>
                  <h3
                    class="mb-1 font-mono text-2xs font-semibold uppercase tracking-wider text-base-content/55"
                  >
                    ACL Rules
                  </h3>
                  <div class="overflow-x-auto rounded border border-base-300">
                    <table class="table table-xs w-full font-mono">
                      <thead>
                        <tr>
                          <th class="w-20 bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Priority</th>
                          <th class="w-24 bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Direction</th>
                          <th class="w-28 bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Action</th>
                          <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Match</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each pg.acls as acl (acl.uuid)}
                          <tr class="border-base-300/60">
                            <td class="text-sm">{acl.priority}</td>
                            <td>
                              <Badge
                                text={directionLabel(acl.direction)}
                                variant={directionVariant(acl.direction)}
                              />
                            </td>
                            <td>
                              <Badge
                                text={acl.action}
                                variant={aclActionVariant(acl.action)}
                              />
                            </td>
                            <td class="break-all text-xs">{acl.match}</td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                </div>
              {:else}
                <span class="font-mono text-sm text-base-content/40">
                  <span class="text-base-content/30">//</span> no ACL rules
                </span>
              {/if}
            </div>
          {/if}
        </section>
      {/each}
    </div>
  {:else if filterText}
    <div class="py-8 text-center font-mono text-sm text-base-content/40">
      <span class="text-base-content/30">//</span> no port groups match the filter
    </div>
  {/if}

  <!-- Standalone ACLs section -->
  {#if filteredStandaloneACLs.length > 0}
    <div class="mt-6">
      <h2
        class="mb-1 font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
      >
        Standalone ACLs
      </h2>
      <p class="mb-3 font-prose text-sm text-base-content/55">
        ACL rules not assigned to any port group
      </p>
      <div class="overflow-x-auto rounded border border-base-300">
        <table class="table table-xs w-full font-mono">
          <thead>
            <tr>
              <th class="w-20 bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Priority</th>
              <th class="w-24 bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Direction</th>
              <th class="w-28 bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Action</th>
              <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Match</th>
            </tr>
          </thead>
          <tbody>
            {#each filteredStandaloneACLs as acl (acl.uuid)}
              <tr class="border-base-300/60">
                <td class="text-sm">{acl.priority}</td>
                <td>
                  <Badge
                    text={directionLabel(acl.direction)}
                    variant={directionVariant(acl.direction)}
                  />
                </td>
                <td>
                  <Badge text={acl.action} variant={aclActionVariant(acl.action)} />
                </td>
                <td class="break-all text-xs">{acl.match}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {:else if !filterText && standaloneACLs.length === 0 && portGroups.length === 0}
    <div class="py-8 text-center font-mono text-sm text-base-content/40">
      <span class="text-base-content/30">//</span> no security policy data available
    </div>
  {/if}
</DataState>
