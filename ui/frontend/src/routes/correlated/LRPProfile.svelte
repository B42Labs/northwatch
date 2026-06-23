<script lang="ts">
  import { getCorrelatedLRP } from '../../lib/api';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import BindingChain from '../../components/profile/BindingChain.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import DataState from '../../components/ui/DataState.svelte';

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
      data = (await getCorrelatedLRP(targetUuid)) as Record<string, unknown>;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load port';
      data = null;
    } finally {
      loading = false;
    }
  }

  let lrp = $derived(
    (data?.logical_router_port ?? {}) as Record<string, unknown>,
  );
</script>

<DataState {loading} {error} empty={!data} emptyMessage="port not found">
  {#if data}
    <EntityHeader
      title={String(lrp.name || 'Port')}
      {uuid}
      type="Logical Router Port"
      breadcrumbs={[
        { label: 'Correlated' },
        { label: 'Logical Routers', href: '/correlated/logical-routers' },
        { label: String(lrp.name || 'port') },
      ]}
      rawHref={`/nb/logical-router-ports/${uuid}`}
    />

    <div class="flex flex-col gap-4">
      <PropertyCard
        title="Port Properties"
        data={lrp}
        exclude={['_uuid', 'name']}
      />

      <div>
        <h2
          class="mb-2 font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
        >
          Binding Chain
        </h2>
        <BindingChain chain={data} />
      </div>
    </div>
  {/if}
</DataState>
