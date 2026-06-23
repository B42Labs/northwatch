<script lang="ts">
  import CorrelatedList from './CorrelatedList.svelte';
  import { listCorrelatedChassis } from '../../lib/api';

  type Row = Record<string, unknown>;

  function transform(data: Row[]): Row[] {
    return data.map((item) => {
      const ch = (item.chassis ?? {}) as Row;
      const encaps = (item.encaps ?? []) as Row[];
      return {
        ...ch,
        encap_count: encaps.length,
        encap_ips: encaps.map((e) => e.ip).join(', '),
      };
    });
  }
</script>

<CorrelatedList
  title="Chassis"
  description="Southbound chassis with their tunnel encapsulations and hypervisor hosts."
  loader={listCorrelatedChassis}
  {transform}
  columns={['_uuid', 'name', 'hostname', 'encap_ips', 'encap_count', 'external_ids']}
  routeBase="/correlated/chassis"
/>
