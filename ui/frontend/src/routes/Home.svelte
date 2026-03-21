<script lang="ts">
  import { link } from '../lib/router';
  import { capabilities, writeEnabled } from '../lib/capabilitiesStore';
  import Badge from '../components/ui/Badge.svelte';
</script>

<div class="mx-auto max-w-3xl">
  <h1 class="mb-2 text-3xl font-bold">Northwatch</h1>
  <p class="mb-6 text-base-content/70">OVN Database Browser & Analyzer</p>

  {#if $capabilities.length > 0}
    <div class="card mb-6 bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-sm">Active Capabilities</h2>
        <div class="flex flex-wrap gap-2">
          {#each $capabilities as cap}
            <Badge text={cap} variant="primary" />
          {/each}
        </div>
      </div>
    </div>
  {/if}

  <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
    <a
      href={link('/correlated/logical-switches')}
      class="card bg-base-100 shadow-sm transition-shadow hover:shadow-md"
    >
      <div class="card-body">
        <h3 class="card-title text-sm">Logical Switches</h3>
        <p class="text-xs text-base-content/60">
          Correlated NB/SB view with enrichment
        </p>
      </div>
    </a>
    <a
      href={link('/correlated/logical-routers')}
      class="card bg-base-100 shadow-sm transition-shadow hover:shadow-md"
    >
      <div class="card-body">
        <h3 class="card-title text-sm">Logical Routers</h3>
        <p class="text-xs text-base-content/60">
          Routers with ports, NATs, and datapaths
        </p>
      </div>
    </a>
    <a
      href={link('/correlated/chassis')}
      class="card bg-base-100 shadow-sm transition-shadow hover:shadow-md"
    >
      <div class="card-body">
        <h3 class="card-title text-sm">Chassis</h3>
        <p class="text-xs text-base-content/60">
          Physical hosts with encaps and port bindings
        </p>
      </div>
    </a>
    {#if $writeEnabled}
      <a
        href={link('/write')}
        class="card bg-base-100 shadow-sm transition-shadow hover:shadow-md"
      >
        <div class="card-body">
          <h3 class="card-title text-sm">Write Operations</h3>
          <p class="text-xs text-base-content/60">
            Create, update, or delete OVN Northbound entities
          </p>
        </div>
      </a>
    {/if}
  </div>
</div>
