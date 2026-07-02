<script lang="ts">
  import { getCorrelatedRouter } from '../../lib/api';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import BindingChain from '../../components/profile/BindingChain.svelte';
  import EnrichmentBadge from '../../components/profile/EnrichmentBadge.svelte';
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
      'nb',
      ['Logical_Router', 'Logical_Router_Port', 'NAT'],
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
      data = (await getCorrelatedRouter(targetUuid)) as unknown as Record<
        string,
        unknown
      >;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load router';
      data = null;
    } finally {
      loading = false;
    }
  }

  let lr = $derived((data?.logical_router ?? {}) as Record<string, unknown>);
  let dp = $derived(
    (data?.datapath_binding ?? null) as Record<string, unknown> | null,
  );
  let ports = $derived((data?.ports ?? []) as Record<string, unknown>[]);
  let nats = $derived((data?.nats ?? []) as Record<string, unknown>[]);
  let enrichment = $derived(
    (lr?.enrichment ?? null) as Record<string, unknown> | null,
  );
</script>

<DataState {loading} {error} empty={!data} emptyMessage="router not found">
  {#if data}
    <EntityHeader
      title={String(lr.name || 'Unnamed Router')}
      {uuid}
      type="Logical Router"
      breadcrumbs={[
        { label: 'Correlated' },
        { label: 'Logical Routers', href: '/correlated/logical-routers' },
        { label: String(lr.name || 'router') },
      ]}
      {enrichment}
      rawHref={`/nb/logical-routers/${uuid}`}
    />

    <div class="flex flex-col gap-4">
      <PropertyCard
        title="Properties"
        data={lr}
        exclude={['_uuid', 'name', 'ports', 'nat', 'enrichment']}
      />

      {#if dp}
        <PropertyCard
          title="Datapath Binding · SB"
          data={dp}
          exclude={['_uuid']}
        />
      {/if}

      {#if nats.length > 0}
        <Card title="NAT Rules" subtitle={String(nats.length)} padding="none">
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
                    >External IP</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >Logical IP</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >External IDs</th
                  >
                  <th
                    class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                    >Enrichment</th
                  >
                </tr>
              </thead>
              <tbody>
                {#each nats as nat (nat._uuid)}
                  <tr class="border-base-300/60">
                    <td>
                      {#if nat.type}<Badge
                          text={String(nat.type)}
                          variant="ghost"
                        />{:else}<span class="text-base-content/40">-</span
                        >{/if}
                    </td>
                    <td class="text-xs">{nat.external_ip || '-'}</td>
                    <td class="text-xs">{nat.logical_ip || '-'}</td>
                    <td><CellRenderer value={nat.external_ids} /></td>
                    <td>
                      {#if nat.enrichment}
                        <EnrichmentBadge
                          data={nat.enrichment as Record<string, unknown>}
                        />
                      {:else}
                        <span class="text-base-content/40">-</span>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </Card>
      {/if}

      {#if ports.length > 0}
        <div>
          <div class="mb-2 flex items-center gap-2">
            <h2
              class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
            >
              Ports
            </h2>
            <span class="font-mono text-2xs text-base-content/40"
              >{ports.length}</span
            >
          </div>
          <div class="flex flex-col gap-2">
            {#each ports as port, i (i)}
              {@const lrp = (port.logical_router_port ?? {}) as Record<
                string,
                unknown
              >}
              <details class="group rounded border border-base-300 bg-base-100">
                <summary
                  class="flex cursor-pointer list-none items-center gap-2 px-3 py-2 font-mono text-sm marker:content-none hover:bg-base-300/30"
                >
                  <span
                    class="select-none text-primary transition-transform group-open:rotate-90"
                    aria-hidden="true">▸</span
                  >
                  <span class="truncate">{lrp.name || lrp._uuid || 'Port'}</span
                  >
                </summary>
                <div class="border-t border-base-300 p-3">
                  <BindingChain chain={port} />
                </div>
              </details>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</DataState>
