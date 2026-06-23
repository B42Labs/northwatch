<script lang="ts">
  import { getCorrelatedSwitch } from '../../lib/api';
  import EntityHeader from '../../components/profile/EntityHeader.svelte';
  import PropertyCard from '../../components/profile/PropertyCard.svelte';
  import BindingChain from '../../components/profile/BindingChain.svelte';
  import EnrichmentBadge from '../../components/profile/EnrichmentBadge.svelte';
  import Badge from '../../components/ui/Badge.svelte';
  import Card from '../../components/ui/Card.svelte';
  import DataState from '../../components/ui/DataState.svelte';
  import { subscribeToTables } from '../../lib/eventStore';

  let { uuid }: { uuid: string } = $props();

  let data: Record<string, unknown> | null = $state(null);
  let loading = $state(true);
  let error = $state('');
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  $effect(() => {
    load(uuid);

    const unsubscribe = subscribeToTables(
      'nb',
      ['Logical_Switch', 'Logical_Switch_Port'],
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
      data = (await getCorrelatedSwitch(targetUuid)) as Record<string, unknown>;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load switch';
      data = null;
    } finally {
      loading = false;
    }
  }

  let sw = $derived((data?.logical_switch ?? {}) as Record<string, unknown>);
  let dp = $derived(
    (data?.datapath_binding ?? null) as Record<string, unknown> | null,
  );
  let ports = $derived((data?.ports ?? []) as Record<string, unknown>[]);
  let enrichment = $derived(
    (sw?.enrichment ?? null) as Record<string, unknown> | null,
  );
</script>

<DataState {loading} {error} empty={!data} emptyMessage="switch not found">
  {#if data}
    <EntityHeader
      title={String(sw.name || 'Unnamed Switch')}
      {uuid}
      type="Logical Switch"
      breadcrumbs={[
        { label: 'Correlated' },
        { label: 'Logical Switches', href: '/correlated/logical-switches' },
        { label: String(sw.name || 'switch') },
      ]}
      {enrichment}
      rawHref={`/nb/logical-switches/${uuid}`}
    />

    <div class="flex flex-col gap-4">
      <PropertyCard
        title="Properties"
        data={sw}
        exclude={['_uuid', 'name', 'ports', 'enrichment']}
      />

      {#if dp}
        <PropertyCard
          title="Datapath Binding · SB"
          data={dp}
          exclude={['_uuid']}
        />
      {/if}

      {#if enrichment}
        <Card title="Enrichment">
          <EnrichmentBadge data={enrichment} />
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
              {@const lsp = (port.logical_switch_port ?? {}) as Record<
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
                  <span class="truncate">{lsp.name || lsp._uuid || 'Port'}</span
                  >
                  {#if lsp.type}<Badge
                      text={String(lsp.type)}
                      variant="ghost"
                    />{/if}
                  {#if lsp.enrichment}
                    <EnrichmentBadge
                      data={lsp.enrichment as Record<string, unknown>}
                    />
                  {/if}
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
