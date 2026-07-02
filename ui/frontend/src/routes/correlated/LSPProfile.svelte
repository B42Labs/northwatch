<script lang="ts">
  import { getCorrelatedLSP } from '../../lib/api';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import BindingChain from '../../components/profile/BindingChain.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import DataState from '../../components/ui/DataState.svelte';

  let { uuid }: { uuid: string } = $props();

  let data = $state<Record<string, unknown> | null>(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    load(uuid);
  });

  async function load(targetUuid: string) {
    loading = true;
    error = '';
    try {
      data = (await getCorrelatedLSP(targetUuid)) as unknown as Record<
        string,
        unknown
      >;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load port';
      data = null;
    } finally {
      loading = false;
    }
  }

  let lsp = $derived(
    (data?.logical_switch_port ?? {}) as Record<string, unknown>,
  );
  let enrichment = $derived(
    (lsp?.enrichment ?? null) as Record<string, unknown> | null,
  );
</script>

<DataState {loading} {error} empty={!data} emptyMessage="port not found">
  {#if data}
    <EntityHeader
      title={String(lsp.name || 'Port')}
      {uuid}
      type="Logical Switch Port"
      breadcrumbs={[
        { label: 'Correlated' },
        { label: 'Logical Switches', href: '/correlated/logical-switches' },
        { label: String(lsp.name || 'port') },
      ]}
      {enrichment}
      rawHref={`/nb/logical-switch-ports/${uuid}`}
    />

    <div class="flex flex-col gap-4">
      <PropertyCard
        title="Port Properties"
        data={lsp}
        exclude={['_uuid', 'name', 'enrichment']}
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
