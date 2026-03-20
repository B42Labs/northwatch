<script lang="ts">
  import { onMount } from 'svelte';
  import { listCorrelatedChassis } from '../../lib/api';
  import { push } from '../../lib/router';
  import DataTable from '../../components/table/DataTable.svelte';
  import LoadingSpinner from '../../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../../components/ui/ErrorAlert.svelte';

  let rows: Record<string, unknown>[] = $state([]);
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      const data = (await listCorrelatedChassis()) as Record<string, unknown>[];
      rows = data.map((item) => {
        const ch = (item.chassis ?? {}) as Record<string, unknown>;
        const encaps = (item.encaps ?? []) as Record<string, unknown>[];
        return {
          ...ch,
          encap_count: encaps.length,
          encap_ips: encaps.map((e) => e.ip).join(', '),
        };
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load chassis';
    } finally {
      loading = false;
    }
  });

  function handleRowClick(row: Record<string, unknown>) {
    const uuid = row._uuid as string;
    if (uuid) push(`/correlated/chassis/${uuid}`);
  }
</script>

<div>
  <h1 class="mb-4 text-xl font-bold">Chassis (Correlated)</h1>

  {#if loading}
    <LoadingSpinner />
  {:else if error}
    <ErrorAlert message={error} />
  {:else}
    <DataTable
      {rows}
      columns={[
        '_uuid',
        'name',
        'hostname',
        'encap_ips',
        'encap_count',
        'external_ids',
      ]}
      onRowClick={handleRowClick}
    />
  {/if}
</div>
