<script lang="ts">
  import CorrelatedList from './CorrelatedList.svelte';
  import { listCorrelatedRouters } from '../../lib/api';

  type Row = Record<string, unknown>;

  function transform(data: Row[]): Row[] {
    return data.map((item) => {
      const lr = (item.logical_router ?? {}) as Row;
      const dp = (item.datapath_binding ?? {}) as Row;
      return { ...lr, datapath_tunnel_key: dp.tunnel_key ?? '-' };
    });
  }
</script>

<CorrelatedList
  title="Logical Routers"
  description="Northbound logical routers joined to their realized Southbound datapath bindings."
  loader={listCorrelatedRouters}
  {transform}
  columns={['_uuid', 'name', 'datapath_tunnel_key', 'external_ids']}
  routeBase="/correlated/logical-routers"
/>
