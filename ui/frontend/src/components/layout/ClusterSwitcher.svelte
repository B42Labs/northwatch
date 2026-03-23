<script lang="ts">
  import {
    clusters,
    activeCluster,
    multiClusterEnabled,
  } from '../../lib/clusterStore';
</script>

{#if $multiClusterEnabled}
  <div class="flex items-center gap-1">
    <span class="text-xs opacity-60">Cluster:</span>
    <select
      class="select select-bordered select-xs"
      value={$activeCluster}
      onchange={(e) => {
        const target = e.target as HTMLSelectElement;
        activeCluster.set(target.value);
      }}
    >
      {#each $clusters as c (c.name)}
        <option value={c.name}>
          {c.label}
          {#if !c.ready}(offline){/if}
        </option>
      {/each}
    </select>
  </div>
{/if}
