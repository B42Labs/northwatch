<script lang="ts">
  import { getCorrelatedPortBinding } from '../../lib/api';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import BindingChain from '../../components/profile/BindingChain.svelte';
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
      data = (await getCorrelatedPortBinding(targetUuid)) as Record<
        string,
        unknown
      >;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load port binding';
      data = null;
    } finally {
      loading = false;
    }
  }

  let pb = $derived((data?.port_binding ?? {}) as Record<string, unknown>);
</script>

<DataState
  {loading}
  {error}
  empty={!data}
  emptyMessage="port binding not found"
>
  {#if data}
    <EntityHeader
      title={String(pb.logical_port || 'Port Binding')}
      {uuid}
      type="Port Binding"
      breadcrumbs={[
        { label: 'Correlated' },
        { label: 'Port Bindings', href: '/sb/port-bindings' },
        { label: String(pb.logical_port || 'port binding') },
      ]}
      rawHref={`/sb/port-bindings/${uuid}`}
    />

    <div class="flex flex-col gap-4">
      <h2
        class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
      >
        Binding Chain
      </h2>
      <BindingChain chain={data} />
    </div>
  {/if}
</DataState>
