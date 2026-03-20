<script lang="ts">
  import { listTable } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
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

  function directionVariant(dir: string): string {
    return dir === 'to-lport' ? 'badge-info' : 'badge-warning';
  }

  function actionVariant(action: string): string {
    if (
      action === 'allow' ||
      action === 'allow-related' ||
      action === 'allow-stateless'
    ) {
      return 'badge-success';
    }
    if (action === 'drop') return 'badge-error';
    if (action === 'reject') return 'badge-warning';
    return 'badge-neutral';
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

<div>
  <!-- Header -->
  <div class="mb-4">
    <h1 class="text-xl font-bold">Security Policy</h1>
    <p class="text-sm text-base-content/60">
      ACL rules and Port Groups — showing how traffic is filtered across logical
      ports
    </p>
  </div>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else}
    <!-- Summary + Filter bar -->
    <div class="mb-4 flex flex-wrap items-center gap-4">
      <div>
        <input
          type="text"
          bind:value={filterText}
          placeholder="Filter by name, port, or match..."
          class="input input-sm input-bordered w-72"
        />
      </div>
      <div
        class="stats stats-horizontal ml-auto border border-base-300 bg-base-100 shadow-sm"
      >
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Port Groups</div>
          <div class="stat-value text-lg">{portGroups.length}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">ACL Rules</div>
          <div class="stat-value text-lg">{totalACLs}</div>
        </div>
        {#if standaloneACLs.length > 0}
          <div class="stat px-4 py-2">
            <div class="stat-title text-xs">Standalone</div>
            <div class="stat-value text-lg">{standaloneACLs.length}</div>
          </div>
        {/if}
      </div>
    </div>

    <!-- Port Group cards -->
    {#if filteredPortGroups.length > 0}
      <div class="flex flex-col gap-4">
        {#each filteredPortGroups as pg (pg.uuid)}
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <!-- Card header -->
            <div class="card-body p-4">
              <button
                class="flex w-full cursor-pointer items-center gap-3 text-left"
                onclick={() => toggleGroup(pg.uuid)}
              >
                <span
                  class="transition-transform duration-200"
                  class:rotate-90={expandedGroups.has(pg.uuid)}
                >
                  &#9654;
                </span>
                <div class="flex-1">
                  <h2 class="text-base font-semibold">{pg.displayName}</h2>
                  {#if pg.displayName !== pg.name}
                    <p class="font-mono text-xs text-base-content/40">
                      {pg.name}
                    </p>
                  {/if}
                </div>
                <div
                  class="flex items-center gap-2 text-sm text-base-content/60"
                >
                  <span class="badge badge-ghost badge-sm">
                    {pg.ports.length} port{pg.ports.length !== 1 ? 's' : ''}
                  </span>
                  <span class="badge badge-ghost badge-sm">
                    {pg.acls.length} ACL{pg.acls.length !== 1 ? 's' : ''}
                  </span>
                </div>
              </button>

              <!-- Expanded content -->
              {#if expandedGroups.has(pg.uuid)}
                <!-- Member ports -->
                {#if pg.ports.length > 0}
                  <div class="mt-3">
                    <h3
                      class="mb-1 text-xs font-semibold uppercase text-base-content/50"
                    >
                      Member Ports
                    </h3>
                    <div class="flex flex-wrap gap-1.5">
                      {#each pg.ports as port (port.uuid)}
                        <span class="badge badge-outline badge-sm font-mono">
                          {port.name}
                        </span>
                      {/each}
                    </div>
                  </div>
                {/if}

                <!-- ACL rules table -->
                {#if pg.acls.length > 0}
                  <div class="mt-3">
                    <h3
                      class="mb-1 text-xs font-semibold uppercase text-base-content/50"
                    >
                      ACL Rules
                    </h3>
                    <div class="overflow-x-auto">
                      <table class="table table-xs w-full">
                        <thead>
                          <tr>
                            <th class="w-20">Priority</th>
                            <th class="w-24">Direction</th>
                            <th class="w-28">Action</th>
                            <th>Match</th>
                          </tr>
                        </thead>
                        <tbody>
                          {#each pg.acls as acl, i (acl.uuid)}
                            <tr class:bg-base-200={i % 2 === 1}>
                              <td class="font-mono text-sm">{acl.priority}</td>
                              <td>
                                <span
                                  class="badge badge-sm {directionVariant(
                                    acl.direction,
                                  )}"
                                >
                                  {directionLabel(acl.direction)}
                                </span>
                              </td>
                              <td>
                                <span
                                  class="badge badge-sm {actionVariant(
                                    acl.action,
                                  )}"
                                >
                                  {acl.action}
                                </span>
                              </td>
                              <td class="break-all font-mono text-xs"
                                >{acl.match}</td
                              >
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    </div>
                  </div>
                {:else}
                  <p class="mt-3 text-sm text-base-content/50">No ACL rules</p>
                {/if}
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {:else if filterText}
      <div class="py-8 text-center text-base-content/50">
        No port groups match the filter
      </div>
    {/if}

    <!-- Standalone ACLs section -->
    {#if filteredStandaloneACLs.length > 0}
      <div class="mt-6">
        <h2 class="mb-2 text-lg font-bold">Standalone ACLs</h2>
        <p class="mb-3 text-sm text-base-content/60">
          ACL rules not assigned to any port group
        </p>
        <div class="overflow-x-auto rounded-lg border border-base-300">
          <table class="table table-xs w-full">
            <thead>
              <tr>
                <th class="w-20">Priority</th>
                <th class="w-24">Direction</th>
                <th class="w-28">Action</th>
                <th>Match</th>
              </tr>
            </thead>
            <tbody>
              {#each filteredStandaloneACLs as acl, i (acl.uuid)}
                <tr class:bg-base-200={i % 2 === 1}>
                  <td class="font-mono text-sm">{acl.priority}</td>
                  <td>
                    <span
                      class="badge badge-sm {directionVariant(acl.direction)}"
                    >
                      {directionLabel(acl.direction)}
                    </span>
                  </td>
                  <td>
                    <span class="badge badge-sm {actionVariant(acl.action)}">
                      {acl.action}
                    </span>
                  </td>
                  <td class="break-all font-mono text-xs">{acl.match}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {:else if !filterText && standaloneACLs.length === 0 && portGroups.length === 0}
      <div class="py-8 text-center text-base-content/50">
        No security policy data available
      </div>
    {/if}
  {/if}
</div>
