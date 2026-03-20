<script lang="ts">
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import { link } from '../lib/router';

  interface NatRule {
    _uuid: string;
    type: string;
    external_ip: string;
    logical_ip: string;
    external_ids: Record<string, string>;
    logical_port: string;
    gateway_port: string;
    [key: string]: unknown;
  }

  interface LogicalRouter {
    _uuid: string;
    name: string;
    nat: string[];
    static_routes: string[];
    [key: string]: unknown;
  }

  interface StaticRoute {
    _uuid: string;
    ip_prefix: string;
    nexthop: string;
    output_port: string;
    [key: string]: unknown;
  }

  interface RouterNatGroup {
    router: LogicalRouter;
    nats: NatRule[];
    routes: StaticRoute[];
  }

  let loading = $state(true);
  let error = $state('');

  let natRules: NatRule[] = $state([]);
  let routers: LogicalRouter[] = $state([]);
  let staticRoutes: StaticRoute[] = $state([]);

  async function fetchJson<T>(path: string): Promise<T> {
    const res = await fetch(path);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const [nats, rts, routes] = await Promise.all([
        fetchJson<NatRule[]>('/api/v1/nb/nats'),
        fetchJson<LogicalRouter[]>('/api/v1/nb/logical-routers'),
        fetchJson<StaticRoute[]>('/api/v1/nb/logical-router-static-routes'),
      ]);
      natRules = nats;
      routers = rts;
      staticRoutes = routes;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load NAT data';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load();
  });

  // Build a map from NAT UUID to NatRule
  let natByUuid = $derived(new Map(natRules.map((n) => [n._uuid, n])));

  // Build a map from static route UUID to StaticRoute
  let routeByUuid = $derived(new Map(staticRoutes.map((r) => [r._uuid, r])));

  // Group NAT rules by router
  let routerGroups = $derived.by((): RouterNatGroup[] => {
    const groups: RouterNatGroup[] = [];
    for (const router of routers) {
      const routerNatUuids = Array.isArray(router.nat) ? router.nat : [];
      const nats = routerNatUuids
        .map((uuid) => natByUuid.get(uuid))
        .filter((n): n is NatRule => n !== undefined);

      const routerRouteUuids = Array.isArray(router.static_routes)
        ? router.static_routes
        : [];
      const routes = routerRouteUuids
        .map((uuid) => routeByUuid.get(uuid))
        .filter((r): r is StaticRoute => r !== undefined);

      if (nats.length > 0 || routes.length > 0) {
        groups.push({ router, nats, routes });
      }
    }
    return groups.sort((a, b) =>
      (a.router.name || '').localeCompare(b.router.name || ''),
    );
  });

  // Summary counts
  let snatCount = $derived(natRules.filter((n) => n.type === 'snat').length);
  let dnatCount = $derived(natRules.filter((n) => n.type === 'dnat').length);
  let dnatAndSnatCount = $derived(
    natRules.filter((n) => n.type === 'dnat_and_snat').length,
  );
  let routersWithNat = $derived(
    routerGroups.filter((g) => g.nats.length > 0).length,
  );

  let searchQuery = $state('');

  let filteredRouterGroups = $derived.by(() => {
    if (!searchQuery) return routerGroups;
    const q = searchQuery.toLowerCase();
    return routerGroups.filter((g) => {
      if ((g.router.name || '').toLowerCase().includes(q)) return true;
      if (g.router._uuid.toLowerCase().includes(q)) return true;
      if (
        g.nats.some(
          (n) =>
            n.external_ip?.toLowerCase().includes(q) ||
            n.logical_ip?.toLowerCase().includes(q) ||
            n.type?.toLowerCase().includes(q),
        )
      )
        return true;
      if (
        g.routes.some(
          (r) =>
            r.ip_prefix?.toLowerCase().includes(q) ||
            r.nexthop?.toLowerCase().includes(q),
        )
      )
        return true;
      return false;
    });
  });

  function typeBadgeClass(type: string): string {
    switch (type) {
      case 'dnat_and_snat':
        return 'bg-purple-600 text-white';
      case 'snat':
        return 'bg-blue-600 text-white';
      case 'dnat':
        return 'bg-green-600 text-white';
      default:
        return 'badge-ghost';
    }
  }

  function typeBadgeLabel(type: string): string {
    switch (type) {
      case 'dnat_and_snat':
        return 'DNAT+SNAT';
      case 'snat':
        return 'SNAT';
      case 'dnat':
        return 'DNAT';
      default:
        return type || '-';
    }
  }

  function typeArrow(type: string): string {
    switch (type) {
      case 'dnat_and_snat':
        return '\u2194';
      case 'snat':
        return '\u2192';
      case 'dnat':
        return '\u2190';
      default:
        return '-';
    }
  }

  function formatPort(value: unknown): string {
    if (!value) return '-';
    if (typeof value === 'string' && value.length > 0) return value;
    return '-';
  }
</script>

<div>
  <!-- Header -->
  <div class="mb-4">
    <h1 class="text-xl font-bold">NAT Overview</h1>
    <p class="text-sm text-base-content/60">
      NAT rules and static routes grouped by logical router
    </p>
  </div>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else}
    <!-- Summary bar -->
    <div class="mb-4 flex flex-wrap items-center gap-4">
      <div>
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Filter by IP, router, type..."
          class="input input-sm input-bordered w-72"
        />
      </div>
      <div
        class="stats stats-horizontal ml-auto border border-base-300 bg-base-100 shadow-sm"
      >
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">NAT Rules</div>
          <div class="stat-value text-lg">{natRules.length}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">SNAT</div>
          <div class="stat-value text-lg">{snatCount}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">DNAT</div>
          <div class="stat-value text-lg">{dnatCount + dnatAndSnatCount}</div>
        </div>
        <div class="stat px-4 py-2">
          <div class="stat-title text-xs">Routers</div>
          <div class="stat-value text-lg">{routersWithNat}</div>
        </div>
      </div>
    </div>

    {#if filteredRouterGroups.length === 0}
      <div class="py-8 text-center text-base-content/50">
        {#if searchQuery}
          No results matching "{searchQuery}"
        {:else}
          No NAT rules or static routes found
        {/if}
      </div>
    {:else}
      <div class="flex flex-col gap-6">
        {#each filteredRouterGroups as group}
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body p-4">
              <!-- Router header -->
              <div class="flex items-center gap-2">
                <span
                  class="inline-block h-3 w-3 rotate-45 rounded-sm bg-green-500 opacity-85"
                ></span>
                <h2 class="card-title text-sm">
                  <a
                    href={link(
                      `/correlated/logical-routers/${group.router._uuid}`,
                    )}
                    class="link-hover link-primary"
                  >
                    {group.router.name || group.router._uuid.slice(0, 8)}
                  </a>
                </h2>
                <span class="text-xs text-base-content/40">
                  {group.router._uuid.slice(0, 8)}
                </span>
                {#if group.nats.length > 0}
                  <span class="badge badge-outline badge-sm">
                    {group.nats.length} NAT {group.nats.length === 1
                      ? 'rule'
                      : 'rules'}
                  </span>
                {/if}
              </div>

              <!-- NAT rules table -->
              {#if group.nats.length > 0}
                <div class="mt-2 overflow-x-auto">
                  <table class="table table-zebra table-xs">
                    <thead>
                      <tr>
                        <th>Type</th>
                        <th>External IP</th>
                        <th class="text-center">Dir</th>
                        <th>Logical IP</th>
                        <th>Logical Port</th>
                        <th>Gateway Port</th>
                      </tr>
                    </thead>
                    <tbody>
                      {#each group.nats as nat}
                        <tr>
                          <td>
                            <span
                              class="badge badge-sm whitespace-nowrap {typeBadgeClass(
                                nat.type,
                              )}"
                            >
                              {typeBadgeLabel(nat.type)}
                            </span>
                          </td>
                          <td class="font-mono text-xs"
                            >{nat.external_ip || '-'}</td
                          >
                          <td class="text-center text-base-content/60"
                            >{typeArrow(nat.type)}</td
                          >
                          <td class="font-mono text-xs"
                            >{nat.logical_ip || '-'}</td
                          >
                          <td class="text-xs">{formatPort(nat.logical_port)}</td
                          >
                          <td class="text-xs">{formatPort(nat.gateway_port)}</td
                          >
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}

              <!-- Static routes -->
              {#if group.routes.length > 0}
                <div class="mt-3">
                  <h3 class="mb-1 text-xs font-semibold text-base-content/60">
                    Static Routes ({group.routes.length})
                  </h3>
                  <div class="overflow-x-auto">
                    <table class="table table-zebra table-xs">
                      <thead>
                        <tr>
                          <th>Prefix</th>
                          <th>Nexthop</th>
                          <th>Output Port</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each group.routes as route}
                          <tr>
                            <td class="font-mono text-xs"
                              >{route.ip_prefix || '-'}</td
                            >
                            <td class="font-mono text-xs"
                              >{route.nexthop || '-'}</td
                            >
                            <td class="text-xs"
                              >{formatPort(route.output_port)}</td
                            >
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
