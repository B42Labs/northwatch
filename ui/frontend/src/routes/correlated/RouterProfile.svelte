<script lang="ts">
  import { getCorrelatedRouter } from '../../lib/api';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import BindingChain from '../../components/profile/BindingChain.svelte';
  import EnrichmentBadge from '../../components/profile/EnrichmentBadge.svelte';
  import CellRenderer from '../../components/table/CellRenderer.svelte';
  import LoadingSpinner from '../../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../../components/ui/ErrorAlert.svelte';
  import { subscribeToTables } from '../../lib/eventStore';

  let { uuid }: { uuid: string } = $props();

  let data: Record<string, unknown> | null = $state(null);
  let loading = $state(true);
  let error = $state('');
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  $effect(() => {
    load(uuid);

    const unsubscribe = subscribeToTables(
      'nb',
      ['Logical_Router', 'Logical_Router_Port', 'NAT'],
      () => {
        if (refetchTimer) clearTimeout(refetchTimer);
        refetchTimer = setTimeout(() => {
          if (!loading) load(uuid);
        }, 300);
      },
    );

    return () => {
      unsubscribe();
      if (refetchTimer) clearTimeout(refetchTimer);
    };
  });

  async function load(targetUuid: string) {
    loading = true;
    error = '';
    try {
      data = (await getCorrelatedRouter(targetUuid)) as Record<string, unknown>;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load router';
      data = null;
    } finally {
      loading = false;
    }
  }

  let lr = $derived((data?.logical_router ?? {}) as Record<string, unknown>);
  let dp = $derived(
    (data?.datapath_binding ?? null) as Record<string, unknown> | null,
  );
  let ports = $derived((data?.ports ?? []) as Record<string, unknown>[]);
  let nats = $derived((data?.nats ?? []) as Record<string, unknown>[]);
  let enrichment = $derived(
    (lr?.enrichment ?? null) as Record<string, unknown> | null,
  );
</script>

{#if loading}
  <LoadingSpinner />
{:else if error}
  <ErrorAlert message={error} />
{:else if data}
  <EntityHeader
    title={String(lr.name || 'Unnamed Router')}
    {uuid}
    type="Logical Router"
    breadcrumbs={[
      { label: 'Logical Routers', href: '/correlated/logical-routers' },
    ]}
    {enrichment}
    rawHref={`/nb/logical-routers/${uuid}`}
  />

  <div class="flex flex-col gap-4">
    <PropertyCard
      title="Properties"
      data={lr}
      exclude={['_uuid', 'name', 'ports', 'nat', 'enrichment']}
    />

    {#if dp}
      <PropertyCard
        title="Datapath Binding (SB)"
        data={dp}
        exclude={['_uuid']}
      />
    {/if}

    {#if nats.length > 0}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4">
          <h2 class="card-title text-sm">NAT Rules ({nats.length})</h2>
          <div class="overflow-x-auto">
            <table class="table table-zebra table-xs">
              <thead>
                <tr>
                  <th>Type</th>
                  <th>External IP</th>
                  <th>Logical IP</th>
                  <th>External IDs</th>
                  <th>Enrichment</th>
                </tr>
              </thead>
              <tbody>
                {#each nats as nat}
                  <tr>
                    <td
                      ><span class="badge badge-ghost badge-sm"
                        >{nat.type || '-'}</span
                      ></td
                    >
                    <td class="font-mono text-xs">{nat.external_ip || '-'}</td>
                    <td class="font-mono text-xs">{nat.logical_ip || '-'}</td>
                    <td
                      ><CellRenderer
                        value={nat.external_ids}
                        column="external_ids"
                      /></td
                    >
                    <td>
                      {#if nat.enrichment}
                        <EnrichmentBadge
                          data={nat.enrichment as Record<string, unknown>}
                        />
                      {:else}
                        <span class="text-base-content/40">-</span>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    {/if}

    {#if ports.length > 0}
      <div>
        <h2 class="mb-2 text-lg font-semibold">Ports ({ports.length})</h2>
        <div class="flex flex-col gap-3">
          {#each ports as port}
            <details class="collapse collapse-arrow bg-base-100 shadow-sm">
              <summary class="collapse-title text-sm font-medium">
                {#if port.logical_router_port}
                  {@const lrp = port.logical_router_port as Record<
                    string,
                    unknown
                  >}
                  {lrp.name || lrp._uuid || 'Port'}
                {:else}
                  Port
                {/if}
              </summary>
              <div class="collapse-content">
                <BindingChain chain={port} />
              </div>
            </details>
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}
