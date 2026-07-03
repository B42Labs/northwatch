<script lang="ts">
  import { link } from '../../lib/router';
  import EnrichmentBadge from './EnrichmentBadge.svelte';
  import Badge from '../ui/Badge.svelte';

  let { chain }: { chain: Record<string, unknown> } = $props();

  let lsp = $derived(
    chain.logical_switch_port as Record<string, unknown> | undefined,
  );
  let lrp = $derived(
    chain.logical_router_port as Record<string, unknown> | undefined,
  );
  let portBinding = $derived(
    chain.port_binding as Record<string, unknown> | undefined,
  );
  let chassis = $derived(chain.chassis as Record<string, unknown> | undefined);
  let datapath = $derived(
    chain.datapath_binding as Record<string, unknown> | undefined,
  );
  let parentSwitch = $derived(
    chain.logical_switch as Record<string, unknown> | undefined,
  );
  let parentRouter = $derived(
    chain.logical_router as Record<string, unknown> | undefined,
  );
  let enrichment = $derived(
    (lsp?.enrichment ?? lrp?.enrichment) as Record<string, unknown> | undefined,
  );
</script>

{#snippet connector()}
  <div class="ml-3 h-3 w-px bg-base-300" aria-hidden="true"></div>
{/snippet}

<div class="flex flex-col gap-0">
  <!-- NB: Logical Switch Port -->
  {#if lsp}
    <div
      class="rounded border border-l-2 border-base-300 border-l-primary bg-base-100 p-3"
    >
      <div class="flex flex-wrap items-center gap-2">
        <Badge text="NB" variant="primary" />
        <span class="font-mono text-sm font-semibold">Logical Switch Port</span>
        {#if lsp.name}<span class="font-mono text-sm text-base-content/80"
            >{lsp.name}</span
          >{/if}
        {#if enrichment}<EnrichmentBadge data={enrichment} />{/if}
      </div>
      {#if lsp._uuid}
        <a
          href={link(`/correlated/logical-switch-ports/${lsp._uuid}`)}
          class="link font-mono text-xs link-primary">{lsp._uuid}</a
        >
      {/if}
      {#if parentSwitch}
        <div class="mt-1 font-mono text-xs text-base-content/55">
          switch: <a
            href={link(`/correlated/logical-switches/${parentSwitch._uuid}`)}
            class="link link-hover">{parentSwitch.name || parentSwitch._uuid}</a
          >
        </div>
      {/if}
    </div>
    {@render connector()}
  {/if}

  <!-- NB: Logical Router Port -->
  {#if lrp}
    <div
      class="rounded border border-l-2 border-base-300 border-l-primary bg-base-100 p-3"
    >
      <div class="flex flex-wrap items-center gap-2">
        <Badge text="NB" variant="primary" />
        <span class="font-mono text-sm font-semibold">Logical Router Port</span>
        {#if lrp.name}<span class="font-mono text-sm text-base-content/80"
            >{lrp.name}</span
          >{/if}
      </div>
      {#if lrp._uuid}
        <a
          href={link(`/correlated/logical-router-ports/${lrp._uuid}`)}
          class="link font-mono text-xs link-primary">{lrp._uuid}</a
        >
      {/if}
      {#if parentRouter}
        <div class="mt-1 font-mono text-xs text-base-content/55">
          router: <a
            href={link(`/correlated/logical-routers/${parentRouter._uuid}`)}
            class="link link-hover">{parentRouter.name || parentRouter._uuid}</a
          >
        </div>
      {/if}
    </div>
    {@render connector()}
  {/if}

  <!-- SB: Port Binding -->
  {#if portBinding}
    <div
      class="rounded border border-l-2 border-base-300 border-l-secondary bg-base-100 p-3"
    >
      <div class="flex flex-wrap items-center gap-2">
        <Badge text="SB" variant="secondary" />
        <span class="font-mono text-sm font-semibold">Port Binding</span>
        {#if portBinding.type}<Badge
            text={String(portBinding.type)}
            variant="ghost"
          />{/if}
      </div>
      <div class="mt-1 font-mono text-xs text-base-content/55">
        {portBinding.logical_port || ''} · key: {portBinding.tunnel_key || '-'}
      </div>
    </div>
    {@render connector()}
  {:else}
    <div
      class="rounded border border-l-2 border-base-300 border-l-warning bg-base-100 p-3"
    >
      <span class="font-mono text-sm text-warning"
        >! no port binding (unbound)</span
      >
    </div>
  {/if}

  <!-- SB: Chassis -->
  {#if chassis}
    <div
      class="rounded border border-l-2 border-base-300 border-l-accent bg-base-100 p-3"
    >
      <div class="flex flex-wrap items-center gap-2">
        <Badge text="SB" variant="accent" />
        <span class="font-mono text-sm font-semibold">Chassis</span>
        <a
          href={link(`/correlated/chassis/${chassis._uuid}`)}
          class="link font-mono text-sm link-hover"
          >{chassis.name || chassis.hostname || chassis._uuid}</a
        >
      </div>
    </div>
    {@render connector()}
  {/if}

  <!-- SB: Datapath -->
  {#if datapath}
    <div
      class="rounded border border-l-2 border-base-300 border-l-info bg-base-100 p-3"
    >
      <div class="flex flex-wrap items-center gap-2">
        <Badge text="SB" variant="info" />
        <span class="font-mono text-sm font-semibold">Datapath Binding</span>
        <span class="font-mono text-xs text-base-content/55"
          >key: {datapath.tunnel_key || '-'}</span
        >
      </div>
    </div>
  {/if}
</div>
