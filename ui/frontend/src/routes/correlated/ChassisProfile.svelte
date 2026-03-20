<script lang="ts">
  import { getCorrelatedChassis } from '../../lib/api';
  import { link, push } from '../../lib/router';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import CellRenderer from '../../components/table/CellRenderer.svelte';
  import LoadingSpinner from '../../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../../components/ui/ErrorAlert.svelte';
  import { subscribeToTables } from '../../lib/eventStore';

  let { uuid }: { uuid: string } = $props();

  let data: Record<string, unknown> | null = $state(null);
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
      data = (await getCorrelatedChassis(targetUuid)) as Record<
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

{#if loading}
  <LoadingSpinner />
{:else if error}
  <ErrorAlert message={error} />
{:else if data}
  <EntityHeader
    title={String(ch.name || ch.hostname || 'Chassis')}
    {uuid}
    type="Chassis"
    breadcrumbs={[{ label: 'Chassis', href: '/correlated/chassis' }]}
    rawHref={`/sb/chassis/${uuid}`}
  />

  <div class="flex flex-col gap-4">
    <PropertyCard
      title="Properties"
      data={ch}
      exclude={['_uuid', 'name', 'encaps']}
    />

    {#if cp}
      <PropertyCard title="Chassis Private" data={cp} exclude={['_uuid']} />
    {/if}

    {#if encaps.length > 0}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4">
          <h2 class="card-title text-sm">Encaps ({encaps.length})</h2>
          <div class="overflow-x-auto">
            <table class="table table-zebra table-xs">
              <thead>
                <tr>
                  <th>Type</th>
                  <th>IP</th>
                  <th>Options</th>
                </tr>
              </thead>
              <tbody>
                {#each encaps as enc}
                  <tr>
                    <td
                      ><span class="badge badge-ghost badge-sm"
                        >{enc.type || '-'}</span
                      ></td
                    >
                    <td class="font-mono text-xs">{enc.ip || '-'}</td>
                    <td><CellRenderer value={enc.options} /></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    {/if}

    {#if portBindings.length > 0}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4">
          <h2 class="card-title text-sm">
            Hosted Ports ({portBindings.length})
          </h2>
          <div class="overflow-x-auto">
            <table class="table table-zebra table-xs">
              <thead>
                <tr>
                  <th>UUID</th>
                  <th>Logical Port</th>
                  <th>Type</th>
                  <th>Tunnel Key</th>
                  <th>MAC</th>
                </tr>
              </thead>
              <tbody>
                {#each portBindings as pb}
                  <tr
                    class="hover cursor-pointer"
                    onclick={() => {
                      const id = pb._uuid as string;
                      if (id) push(`/correlated/port-bindings/${id}`);
                    }}
                  >
                    <td>
                      <a
                        href={link(`/correlated/port-bindings/${pb._uuid}`)}
                        class="link link-primary font-mono text-xs"
                      >
                        {String(pb._uuid).slice(0, 8)}
                      </a>
                    </td>
                    <td class="font-mono text-xs">{pb.logical_port || '-'}</td>
                    <td
                      >{#if pb.type}<span class="badge badge-ghost badge-sm"
                          >{pb.type}</span
                        >{:else}-{/if}</td
                    >
                    <td class="font-mono text-xs">{pb.tunnel_key || '-'}</td>
                    <td class="max-w-xs truncate font-mono text-xs"
                      >{Array.isArray(pb.mac)
                        ? pb.mac.join(', ')
                        : pb.mac || '-'}</td
                    >
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}
