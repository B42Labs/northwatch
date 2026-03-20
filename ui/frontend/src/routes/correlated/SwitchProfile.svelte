<script lang="ts">
  import { getCorrelatedSwitch } from '../../lib/api';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import BindingChain from '../../components/profile/BindingChain.svelte';
  import EnrichmentBadge from '../../components/profile/EnrichmentBadge.svelte';
  import LoadingSpinner from '../../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../../components/ui/ErrorAlert.svelte';

  let { uuid }: { uuid: string } = $props();

  let data: Record<string, unknown> | null = $state(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    load(uuid);
  });

  async function load(targetUuid: string) {
    loading = true;
    error = '';
    try {
      data = (await getCorrelatedSwitch(targetUuid)) as Record<string, unknown>;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load switch';
      data = null;
    } finally {
      loading = false;
    }
  }

  let sw = $derived((data?.logical_switch ?? {}) as Record<string, unknown>);
  let dp = $derived(
    (data?.datapath_binding ?? null) as Record<string, unknown> | null,
  );
  let ports = $derived((data?.ports ?? []) as Record<string, unknown>[]);
  let enrichment = $derived(
    (sw?.enrichment ?? null) as Record<string, unknown> | null,
  );
</script>

{#if loading}
  <LoadingSpinner />
{:else if error}
  <ErrorAlert message={error} />
{:else if data}
  <EntityHeader
    title={String(sw.name || 'Unnamed Switch')}
    {uuid}
    type="Logical Switch"
    breadcrumbs={[
      { label: 'Logical Switches', href: '/correlated/logical-switches' },
    ]}
    {enrichment}
    rawHref={`/nb/logical-switches/${uuid}`}
  />

  <div class="flex flex-col gap-4">
    <PropertyCard
      title="Properties"
      data={sw}
      exclude={['_uuid', 'name', 'ports', 'enrichment']}
    />

    {#if dp}
      <PropertyCard
        title="Datapath Binding (SB)"
        data={dp}
        exclude={['_uuid']}
      />
    {/if}

    {#if enrichment}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4">
          <h2 class="card-title text-sm">Enrichment</h2>
          <EnrichmentBadge data={enrichment} />
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
                {#if port.logical_switch_port}
                  {@const lsp = port.logical_switch_port as Record<
                    string,
                    unknown
                  >}
                  {lsp.name || lsp._uuid || 'Port'}
                  {#if lsp.type}
                    <span class="badge badge-ghost badge-sm ml-2"
                      >{lsp.type}</span
                    >
                  {/if}
                  {#if lsp.enrichment}
                    <span class="ml-2">
                      <EnrichmentBadge
                        data={lsp.enrichment as Record<string, unknown>}
                      />
                    </span>
                  {/if}
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
