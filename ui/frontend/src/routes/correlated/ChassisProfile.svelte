<script lang="ts">
  import { getCorrelatedChassis } from '../../lib/api';
  import { link, push } from '../../lib/router';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import CellRenderer from '../../components/table/CellRenderer.svelte';
  import Badge from '../../components/ui/Badge.svelte';
  import Card from '../../components/ui/Card.svelte';
  import DataState from '../../components/ui/DataState.svelte';
  import { subscribeToTables } from '../../lib/eventStore';

  let { uuid }: { uuid: string } = $props();

  let data = $state<Record<string, unknown> | null>(null);
  let loading = $state(true);
  let error = $state('');
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  $effect(() => {
    load(uuid);

    const unsubscribe = subscribeToTables(
      'sb',
      ['Chassis', 'Chassis_Private', 'Encap', 'Port_Binding'],
      () => {
        if (refetchTimer) clearTimeout(refetchTimer);
        refetchTimer = setTimeout(() => {
          if (!loading) load(uuid);
        }, 300);
      },
    );

    return () => {
      unsubscribe();
      if (refetchTimer) clearTimeout(refetchTimer);
    };
  });

  async function load(targetUuid: string) {
    loading = true;
    error = '';
    try {
      data = (await getCorrelatedChassis(targetUuid)) as unknown as Record<
        string,
        unknown
      >;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load chassis';
      data = null;
    } finally {
      loading = false;
    }
  }

  let ch = $derived((data?.chassis ?? {}) as Record<string, unknown>);
  let cp = $derived(
    (data?.chassis_private ?? null) as Record<string, unknown> | null,
  );
  let encaps = $derived((data?.encaps ?? []) as Record<string, unknown>[]);
  let portBindings = $derived(
    (data?.port_bindings ?? []) as Record<string, unknown>[],
  );
</script>

<DataState {loading} {error} empty={!data} emptyMessage="chassis not found">
  {#if data}
    <EntityHeader
      title={String(ch.name || ch.hostname || 'Chassis')}
      {uuid}
      type="Chassis"
      breadcrumbs={[
        { label: 'Correlated' },
        { label: 'Chassis', href: '/correlated/chassis' },
        { label: String(ch.name || ch.hostname || 'chassis') },
      ]}
      rawHref={`/sb/chassis/${uuid}`}
    />

    <div class="flex flex-col gap-4">
      <PropertyCard
        title="Properties"
        data={ch}
        exclude={['_uuid', 'name', 'encaps']}
      />

      {#if cp}
        <PropertyCard
          title="Chassis Private · SB"
          data={cp}
          exclude={['_uuid']}
        />
      {/if}

      {#if encaps.length > 0}
        <Card title="Encaps" subtitle={String(encaps.length)} padding="none">
          <div class="overflow-x-auto rounded border border-base-300">
            <table class="table table-xs font-mono">
              <thead>
                <tr>
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >Type</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >IP</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >Options</th
                  >
                </tr>
              </thead>
              <tbody>
                {#each encaps as enc (enc._uuid)}
                  <tr class="border-base-300/60">
                    <td>
                      {#if enc.type}<Badge
                          text={String(enc.type)}
                          variant="ghost"
                        />{:else}<span class="text-base-content/40">-</span
                        >{/if}
                    </td>
                    <td class="text-xs">{enc.ip || '-'}</td>
                    <td><CellRenderer value={enc.options} /></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </Card>
      {/if}

      {#if portBindings.length > 0}
        <Card
          title="Hosted Ports"
          subtitle={String(portBindings.length)}
          padding="none"
        >
          <div class="overflow-x-auto rounded border border-base-300">
            <table class="table table-xs font-mono">
              <thead>
                <tr>
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >UUID</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >Logical Port</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >Type</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >Tunnel Key</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >MAC</th
                  >
                </tr>
              </thead>
              <tbody>
                {#each portBindings as pb (pb._uuid)}
                  <tr
                    class="cursor-pointer border-base-300/60 hover:bg-base-300/40"
                    onclick={() => {
                      const id = pb._uuid as string;
                      if (id) push(`/correlated/port-bindings/${id}`);
                    }}
                  >
                    <td>
                      <a
                        href={link(`/correlated/port-bindings/${pb._uuid}`)}
                        class="link link-primary text-xs"
                      >
                        {String(pb._uuid).slice(0, 8)}
                      </a>
                    </td>
                    <td class="text-xs">{pb.logical_port || '-'}</td>
                    <td
                      >{#if pb.type}<Badge
                          text={String(pb.type)}
                          variant="ghost"
                        />{:else}-{/if}</td
                    >
                    <td class="text-xs">{pb.tunnel_key || '-'}</td>
                    <td class="max-w-xs truncate text-xs"
                      >{Array.isArray(pb.mac)
                        ? pb.mac.join(', ')
                        : pb.mac || '-'}</td
                    >
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </Card>
      {/if}
    </div>
  {/if}
</DataState>
