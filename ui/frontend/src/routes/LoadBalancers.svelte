<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Card from '../components/ui/Card.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import FilterInput from '../components/ui/FilterInput.svelte';
  import type { Variant } from '../lib/status';

  interface LBBackend {
    address: string;
    status?: string;
  }
  interface LBVIP {
    vip: string;
    backends: LBBackend[];
  }
  interface LBView {
    uuid: string;
    name: string;
    protocol?: string;
    vips: LBVIP[];
    routers: string[];
    switches: string[];
    external_ids?: Record<string, string>;
  }
  interface LBResponse {
    total: number;
    load_balancers: LBView[];
  }

  let data: LBResponse | null = $state(null);
  let loading = $state(true);
  let error = $state('');
  let searchQuery = $state('');

  let filtered = $derived(
    (data?.load_balancers ?? []).filter((lb) => {
      if (!searchQuery) return true;
      const q = searchQuery.toLowerCase();
      return (
        lb.name.toLowerCase().includes(q) ||
        lb.uuid.toLowerCase().includes(q) ||
        lb.vips.some((v) => v.vip.includes(q))
      );
    }),
  );

  function statusVariant(s?: string): Variant {
    if (s === 'online') return 'success';
    if (s === 'offline' || s === 'error') return 'error';
    return 'ghost';
  }

  async function load() {
    loading = true;
    error = '';
    try {
      data = await get<LBResponse>('/api/v1/topology/load-balancers');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  onMount(() => load());
</script>

<PageHeader
  eyebrow="Traffic"
  title="Load Balancers"
  description="VIP to backend mappings with health check status."
/>

<DataState {loading} {error} empty={!data}>
  {#if data}
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <FilterInput
        bind:value={searchQuery}
        placeholder="filter by name, UUID, or VIP…"
        width="w-72"
      />
      <span class="font-mono text-xs text-base-content/55">
        <span class="text-base-content/80">{filtered.length}</span> / {data.total}
        load balancers
      </span>
    </div>

    <div class="flex flex-col gap-4">
      {#each filtered as lb (lb.uuid)}
        <Card
          title={lb.name || lb.uuid.slice(0, 8)}
          subtitle={lb.uuid.slice(0, 8)}
        >
          {#if lb.protocol || lb.routers.length > 0 || lb.switches.length > 0}
            <div class="mb-2 flex flex-wrap gap-1">
              {#if lb.protocol}
                <Badge text={lb.protocol} variant="ghost" />
              {/if}
              {#each lb.routers as r (r)}
                <Badge text="router: {r}" variant="primary" />
              {/each}
              {#each lb.switches as s (s)}
                <Badge text="switch: {s}" variant="secondary" />
              {/each}
            </div>
          {/if}

          {#if lb.vips.length > 0}
            <div class="flex flex-col gap-2">
              {#each lb.vips as vip (vip.vip)}
                <div class="rounded border border-base-300 bg-base-200/60 p-2">
                  <div class="mb-1 font-mono text-sm font-semibold">
                    {vip.vip}
                  </div>
                  <div class="flex flex-wrap gap-1">
                    {#each vip.backends as backend (backend.address)}
                      <Badge
                        text={backend.status
                          ? `${backend.address} (${backend.status})`
                          : backend.address}
                        variant={statusVariant(backend.status)}
                      />
                    {/each}
                    {#if vip.backends.length === 0}
                      <span class="font-mono text-sm text-base-content/40">
                        <span class="text-base-content/30">//</span> no backends
                      </span>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </Card>
      {/each}

      {#if filtered.length === 0}
        <div class="py-8 text-center font-mono text-sm text-base-content/40">
          <span class="text-base-content/30">//</span> no load balancers found
        </div>
      {/if}
    </div>
  {/if}
</DataState>
