<script lang="ts">
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Card from '../components/ui/Card.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import type { Variant } from '../lib/status';
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

  function typeVariant(type: string): Variant {
    switch (type) {
      case 'dnat_and_snat':
        return 'secondary';
      case 'snat':
        return 'info';
      case 'dnat':
        return 'success';
      default:
        return 'ghost';
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

<PageHeader
  eyebrow="Routing"
  title="NAT Overview"
  description="NAT rules and static routes grouped by logical router."
>
  {#snippet actions()}
    <StatTiles
      tiles={[
        { label: 'NAT Rules', value: natRules.length },
        { label: 'SNAT', value: snatCount, variant: 'info' },
        { label: 'DNAT', value: dnatCount + dnatAndSnatCount, variant: 'success' },
        { label: 'Routers', value: routersWithNat },
      ]}
    />
  {/snippet}
</PageHeader>

<DataState {loading} {error}>
  <!-- Filter bar -->
  <div class="mb-3 flex flex-wrap items-center gap-2">
    <FilterInput
      bind:value={searchQuery}
      placeholder="filter by IP, router, type…"
      width="w-72"
    />
  </div>

  {#if filteredRouterGroups.length === 0}
    <div class="py-8 text-center font-mono text-sm text-base-content/40">
      <span class="text-base-content/30">//</span>
      {#if searchQuery}
        no results matching "{searchQuery}"
      {:else}
        no NAT rules or static routes found
      {/if}
    </div>
  {:else}
    <div class="flex flex-col gap-4">
      {#each filteredRouterGroups as group (group.router._uuid)}
        <Card accent="success" subtitle={group.router._uuid.slice(0, 8)}>
          {#snippet actions()}
            {#if group.nats.length > 0}
              <Badge
                text="{group.nats.length} NAT {group.nats.length === 1
                  ? 'rule'
                  : 'rules'}"
                variant="neutral"
                outline
              />
            {/if}
          {/snippet}

          <!-- Router header -->
          <h2 class="mb-2 font-mono text-sm font-semibold">
            <a
              href={link(`/correlated/logical-routers/${group.router._uuid}`)}
              class="link-hover link-primary"
            >
              {group.router.name || group.router._uuid.slice(0, 8)}
            </a>
          </h2>

          <!-- NAT rules table -->
          {#if group.nats.length > 0}
            <div class="overflow-x-auto rounded border border-base-300">
              <table class="table table-xs font-mono">
                <thead>
                  <tr>
                    <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Type</th>
                    <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">External IP</th>
                    <th class="bg-base-200 text-center text-2xs uppercase tracking-wider text-base-content/55">Dir</th>
                    <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Logical IP</th>
                    <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Logical Port</th>
                    <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Gateway Port</th>
                  </tr>
                </thead>
                <tbody>
                  {#each group.nats as nat (nat._uuid)}
                    <tr class="border-base-300/60">
                      <td>
                        <Badge
                          text={typeBadgeLabel(nat.type)}
                          variant={typeVariant(nat.type)}
                        />
                      </td>
                      <td class="text-xs">{nat.external_ip || '-'}</td>
                      <td class="text-center text-base-content/60"
                        >{typeArrow(nat.type)}</td
                      >
                      <td class="text-xs">{nat.logical_ip || '-'}</td>
                      <td class="text-xs text-base-content/70">{formatPort(nat.logical_port)}</td>
                      <td class="text-xs text-base-content/70">{formatPort(nat.gateway_port)}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Static routes -->
          {#if group.routes.length > 0}
            <div class="mt-3">
              <h3
                class="mb-1 font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
              >
                Static Routes ({group.routes.length})
              </h3>
              <div class="overflow-x-auto rounded border border-base-300">
                <table class="table table-xs font-mono">
                  <thead>
                    <tr>
                      <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Prefix</th>
                      <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Nexthop</th>
                      <th class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55">Output Port</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each group.routes as route (route._uuid)}
                      <tr class="border-base-300/60">
                        <td class="text-xs">{route.ip_prefix || '-'}</td>
                        <td class="text-xs">{route.nexthop || '-'}</td>
                        <td class="text-xs text-base-content/70">{formatPort(route.output_port)}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          {/if}
        </Card>
      {/each}
    </div>
  {/if}
</DataState>
