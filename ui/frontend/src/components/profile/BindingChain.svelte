<script lang="ts">
  import { link } from '../../lib/router';
  import EnrichmentBadge from './EnrichmentBadge.svelte';

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

<div class="flex flex-col gap-2">
  <!-- NB source -->
  {#if lsp}
    <div class="card border-l-4 border-primary bg-base-100 shadow-sm">
      <div class="card-body p-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="badge badge-primary badge-sm">NB</span>
          <span class="text-sm font-semibold">Logical Switch Port</span>
          {#if lsp.name}
            <span class="font-mono text-sm">{lsp.name}</span>
          {/if}
          {#if enrichment}
            <EnrichmentBadge data={enrichment} />
          {/if}
        </div>
        {#if lsp._uuid}
          <a
            href={link(`/correlated/logical-switch-ports/${lsp._uuid}`)}
            class="link link-primary font-mono text-xs"
          >
            {lsp._uuid}
          </a>
        {/if}
        {#if parentSwitch}
          <div class="text-xs text-base-content/60">
            Switch: <a
              href={link(`/correlated/logical-switches/${parentSwitch._uuid}`)}
              class="link-hover link"
              >{parentSwitch.name || parentSwitch._uuid}</a
            >
          </div>
        {/if}
      </div>
    </div>
    <div class="flex justify-center text-base-content/30">|</div>
  {/if}

  {#if lrp}
    <div class="card border-l-4 border-primary bg-base-100 shadow-sm">
      <div class="card-body p-3">
        <div class="flex items-center gap-2">
          <span class="badge badge-primary badge-sm">NB</span>
          <span class="text-sm font-semibold">Logical Router Port</span>
          {#if lrp.name}
            <span class="font-mono text-sm">{lrp.name}</span>
          {/if}
        </div>
        {#if lrp._uuid}
          <a
            href={link(`/correlated/logical-router-ports/${lrp._uuid}`)}
            class="link link-primary font-mono text-xs"
          >
            {lrp._uuid}
          </a>
        {/if}
        {#if parentRouter}
          <div class="text-xs text-base-content/60">
            Router: <a
              href={link(`/correlated/logical-routers/${parentRouter._uuid}`)}
              class="link-hover link"
              >{parentRouter.name || parentRouter._uuid}</a
            >
          </div>
        {/if}
      </div>
    </div>
    <div class="flex justify-center text-base-content/30">|</div>
  {/if}

  <!-- Port Binding -->
  {#if portBinding}
    <div class="card border-l-4 border-secondary bg-base-100 shadow-sm">
      <div class="card-body p-3">
        <div class="flex items-center gap-2">
          <span class="badge badge-secondary badge-sm">SB</span>
          <span class="text-sm font-semibold">Port Binding</span>
          {#if portBinding.type}
            <span class="badge badge-ghost badge-sm">{portBinding.type}</span>
          {/if}
        </div>
        <div class="font-mono text-xs text-base-content/60">
          {portBinding.logical_port || ''} / key: {portBinding.tunnel_key ||
            '-'}
        </div>
      </div>
    </div>
    <div class="flex justify-center text-base-content/30">|</div>
  {:else}
    <div class="card border-l-4 border-warning bg-base-100 shadow-sm">
      <div class="card-body p-3">
        <span class="text-sm text-warning">No Port Binding (unbound)</span>
      </div>
    </div>
  {/if}

  <!-- Chassis -->
  {#if chassis}
    <div class="card border-l-4 border-accent bg-base-100 shadow-sm">
      <div class="card-body p-3">
        <div class="flex items-center gap-2">
          <span class="badge badge-accent badge-sm">SB</span>
          <span class="text-sm font-semibold">Chassis</span>
          <a
            href={link(`/correlated/chassis/${chassis._uuid}`)}
            class="link-hover link font-mono text-sm"
          >
            {chassis.name || chassis.hostname || chassis._uuid}
          </a>
        </div>
      </div>
    </div>
    <div class="flex justify-center text-base-content/30">|</div>
  {/if}

  <!-- Datapath -->
  {#if datapath}
    <div class="card border-l-4 border-info bg-base-100 shadow-sm">
      <div class="card-body p-3">
        <div class="flex items-center gap-2">
          <span class="badge badge-info badge-sm">SB</span>
          <span class="text-sm font-semibold">Datapath Binding</span>
          <span class="font-mono text-xs"
            >key: {datapath.tunnel_key || '-'}</span
          >
        </div>
      </div>
    </div>
  {/if}
</div>
