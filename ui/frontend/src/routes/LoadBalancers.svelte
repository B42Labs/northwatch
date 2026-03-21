<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';

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

  function statusBadge(s?: string): string {
    if (s === 'online') return 'badge-success';
    if (s === 'offline' || s === 'error') return 'badge-error';
    return 'badge-ghost';
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

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Load Balancers</h1>
    <p class="text-sm text-base-content/60">
      VIP to backend mappings with health check status
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if loading}
    <LoadingSpinner />
  {:else if data}
    <div class="mb-4 flex items-center gap-3">
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Search by name, UUID, or VIP..."
        class="input input-sm input-bordered w-72"
      />
      <span class="text-sm text-base-content/50"
        >{filtered.length} of {data.total} load balancers</span
      >
    </div>

    <div class="flex flex-col gap-4">
      {#each filtered as lb}
        <div class="card border border-base-300 bg-base-100 shadow-sm">
          <div class="card-body p-4">
            <div class="flex items-center justify-between">
              <div>
                <h2 class="card-title text-base">
                  {lb.name || lb.uuid.slice(0, 8)}
                </h2>
                <div class="flex gap-1">
                  {#if lb.protocol}
                    <span class="badge badge-ghost badge-xs">{lb.protocol}</span
                    >
                  {/if}
                  {#each lb.routers as r}
                    <span class="badge badge-primary badge-xs">router: {r}</span
                    >
                  {/each}
                  {#each lb.switches as s}
                    <span class="badge badge-secondary badge-xs"
                      >switch: {s}</span
                    >
                  {/each}
                </div>
              </div>
              <span class="text-xs text-base-content/40"
                >{lb.uuid.slice(0, 8)}</span
              >
            </div>

            {#if lb.vips.length > 0}
              <div class="mt-3">
                {#each lb.vips as vip}
                  <div class="mb-2 rounded bg-base-200 p-2">
                    <div class="mb-1 font-mono text-sm font-semibold">
                      {vip.vip}
                    </div>
                    <div class="flex flex-wrap gap-1">
                      {#each vip.backends as backend}
                        <span
                          class="badge badge-sm {statusBadge(backend.status)}"
                        >
                          {backend.address}
                          {#if backend.status}({backend.status}){/if}
                        </span>
                      {/each}
                      {#if vip.backends.length === 0}
                        <span class="text-xs text-base-content/40"
                          >No backends</span
                        >
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      {/each}

      {#if filtered.length === 0}
        <div class="py-8 text-center text-sm text-base-content/40">
          No load balancers found
        </div>
      {/if}
    </div>
  {/if}
</div>
