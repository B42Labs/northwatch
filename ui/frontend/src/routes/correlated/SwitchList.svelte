<script lang="ts">
  import { onMount } from 'svelte';
  import { listCorrelatedSwitches } from '../../lib/api';
  import { push } from '../../lib/router';
  import DataTable from '../../components/table/DataTable.svelte';
  import LoadingSpinner from '../../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../../components/ui/ErrorAlert.svelte';

  let rows: Record<string, unknown>[] = $state([]);
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      const data = (await listCorrelatedSwitches()) as Record<
        string,
        unknown
      >[];
      rows = data.map((item) => {
        const sw = (item.logical_switch ?? {}) as Record<string, unknown>;
        const dp = (item.datapath_binding ?? {}) as Record<string, unknown>;
        return {
          ...sw,
          datapath_tunnel_key: dp.tunnel_key ?? '-',
        };
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load switches';
    } finally {
      loading = false;
    }
  });

  function handleRowClick(row: Record<string, unknown>) {
    const uuid = row._uuid as string;
    if (uuid) push(`/correlated/logical-switches/${uuid}`);
  }
</script>

<div>
  <h1 class="mb-4 text-xl font-bold">Logical Switches (Correlated)</h1>

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
