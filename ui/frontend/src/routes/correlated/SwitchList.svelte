<script lang="ts">
  import CorrelatedList from './CorrelatedList.svelte';
  import { listCorrelatedSwitches } from '../../lib/api';

  type Row = Record<string, unknown>;

  function transform(data: Row[]): Row[] {
    return data.map((item) => {
      const sw = (item.logical_switch ?? {}) as Row;
      const dp = (item.datapath_binding ?? {}) as Row;
      return { ...sw, datapath_tunnel_key: dp.tunnel_key ?? '-' };
    });
  }
</script>

<CorrelatedList
  title="Logical Switches"
  description="Northbound logical switches joined to their realized Southbound datapath bindings."
  loader={listCorrelatedSwitches}
  {transform}
  columns={['_uuid', 'name', 'datapath_tunnel_key', 'external_ids']}
  routeBase="/correlated/logical-switches"
/>
