<script lang="ts">
  import {
    clusters,
    activeCluster,
    multiClusterEnabled,
  } from '../../lib/clusterStore';
</script>

{#if $multiClusterEnabled}
  <label class="flex items-center gap-1.5">
    <span
      class="hidden font-mono text-2xs uppercase tracking-wider text-base-content/45 sm:inline"
      >cluster</span
    >
    <select
      class="select select-bordered select-xs bg-base-200/60 font-mono"
      value={$activeCluster}
      onchange={(e) => activeCluster.set((e.target as HTMLSelectElement).value)}
      aria-label="Active cluster"
    >
      {#each $clusters as c (c.name)}
        <option value={c.name}>{c.label}{c.ready ? '' : ' (offline)'}</option>
      {/each}
    </select>
  </label>
{/if}
