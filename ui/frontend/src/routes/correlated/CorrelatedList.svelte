<script lang="ts">
  import { onMount } from 'svelte';
  import { push } from '../../lib/router';
  import DataTable from '../../components/table/DataTable.svelte';
  import DataState from '../../components/ui/DataState.svelte';
  import PageHeader from '../../components/ui/PageHeader.svelte';

  type Row = Record<string, unknown>;

  // One generic correlated-list page. SwitchList / RouterList / ChassisList
  // were ~95% identical copies; they now just pass config into this.
  let {
    title,
    description,
    loader,
    transform,
    columns,
    routeBase,
  }: {
    title: string;
    description?: string;
    loader: () => Promise<unknown>;
    transform: (data: Row[]) => Row[];
    columns: string[];
    routeBase: string;
  } = $props();

  let rows: Row[] = $state([]);
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      const data = (await loader()) as Row[];
      rows = transform(data);
    } catch (e) {
      error = e instanceof Error ? e.message : `Failed to load ${title}`;
    } finally {
      loading = false;
    }
  });

  function handleRowClick(row: Row) {
    const uuid = row._uuid as string;
    if (uuid) push(`${routeBase}/${uuid}`);
  }
</script>

<PageHeader eyebrow="Correlated" {title} {description} />

<DataState
  {loading}
  {error}
  empty={rows.length === 0}
  emptyMessage="no {title.toLowerCase()} found"
>
  <DataTable {rows} {columns} onRowClick={handleRowClick} />
</DataState>
