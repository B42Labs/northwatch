<script lang="ts">
  import { onMount } from 'svelte';
  import { listCorrelatedRouters } from '../../lib/api';
  import { push } from '../../lib/router';
  import DataTable from '../../components/table/DataTable.svelte';
  import LoadingSpinner from '../../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../../components/ui/ErrorAlert.svelte';

  let rows: Record<string, unknown>[] = $state([]);
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      const data = (await listCorrelatedRouters()) as Record<string, unknown>[];
      rows = data.map((item) => {
        const lr = (item.logical_router ?? {}) as Record<string, unknown>;
        const dp = (item.datapath_binding ?? {}) as Record<string, unknown>;
        return {
          ...lr,
          datapath_tunnel_key: dp.tunnel_key ?? '-',
        };
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load routers';
    } finally {
      loading = false;
    }
  });

  function handleRowClick(row: Record<string, unknown>) {
    const uuid = row._uuid as string;
    if (uuid) push(`/correlated/logical-routers/${uuid}`);
  }
</script>

<div>
  <h1 class="mb-4 text-xl font-bold">Logical Routers (Correlated)</h1>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else}
    <DataTable
      {rows}
      columns={['_uuid', 'name', 'datapath_tunnel_key', 'external_ids']}
      onRowClick={handleRowClick}
    />
  {/if}
</div>
